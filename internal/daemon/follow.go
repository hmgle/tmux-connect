package daemon

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hmgle/tmux-connect/internal/tagb"
)

type FollowManager struct {
	service       paneService
	replyBus      *ReplyBus
	initialLines  int
	minInterval   time.Duration
	maxMessageLen int
	debugWriter   io.Writer

	mu       sync.Mutex
	debugMu  sync.Mutex
	sessions map[string]*followSession
}

type FollowOptions struct {
	MinInterval time.Duration
}

type followSession struct {
	chat        ChatRef
	paneKey     string
	minInterval time.Duration
	cancel      context.CancelFunc
}

func NewFollowManager(service paneService, replyBus *ReplyBus, initialLines int) *FollowManager {
	return &FollowManager{
		service:       service,
		replyBus:      replyBus,
		initialLines:  initialLines,
		minInterval:   700 * time.Millisecond,
		maxMessageLen: 3500,
		sessions:      make(map[string]*followSession),
	}
}

func (m *FollowManager) Enable(ctx context.Context, chat ChatRef, paneKey string) error {
	return m.EnableWithOptions(ctx, chat, paneKey, FollowOptions{})
}

func (m *FollowManager) SetDebugWriter(w io.Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.debugWriter = w
}

func (m *FollowManager) EnableWithOptions(ctx context.Context, chat ChatRef, paneKey string, opts FollowOptions) error {
	chat = chat.Normalized()
	paneKey = strings.TrimSpace(paneKey)
	if paneKey == "" {
		return fmt.Errorf("pane key is required")
	}
	opts = m.normalizeOptions(opts)

	stream, err := m.service.OpenStream(ctx, paneKey, m.initialLines)
	if err != nil {
		return err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	session := &followSession{
		chat:        chat,
		paneKey:     stream.Pane.Target.PaneKey(),
		minInterval: opts.MinInterval,
		cancel:      cancel,
	}
	chatKey := chat.Key()

	m.mu.Lock()
	if existing := m.sessions[chatKey]; existing != nil {
		existing.cancel()
	}
	m.sessions[chatKey] = session
	m.mu.Unlock()

	go m.run(runCtx, session, stream)
	return nil
}

func (m *FollowManager) Options(chatKey string) FollowOptions {
	m.mu.Lock()
	defer m.mu.Unlock()

	session := m.sessions[chatKey]
	if session == nil {
		return FollowOptions{MinInterval: m.minInterval}
	}
	return FollowOptions{MinInterval: session.minInterval}
}

func (m *FollowManager) Disable(chatKey string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	session := m.sessions[chatKey]
	if session == nil {
		return false
	}
	delete(m.sessions, chatKey)
	session.cancel()
	return true
}

func (m *FollowManager) StopPane(paneKey string) {
	paneKey = strings.TrimSpace(paneKey)
	m.mu.Lock()
	defer m.mu.Unlock()
	for chatKey, session := range m.sessions {
		if session.paneKey != paneKey {
			continue
		}
		delete(m.sessions, chatKey)
		session.cancel()
	}
}

func (m *FollowManager) IsEnabled(chatKey string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[chatKey] != nil
}

func (m *FollowManager) CurrentPane(chatKey string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if session := m.sessions[chatKey]; session != nil {
		return session.paneKey
	}
	return ""
}

func (m *FollowManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for chatKey, session := range m.sessions {
		delete(m.sessions, chatKey)
		session.cancel()
	}
}

func (m *FollowManager) run(ctx context.Context, session *followSession, stream tagb.PaneStream) {
	defer stream.Subscription.Close()
	defer m.removeSession(session.chat.Key(), session.paneKey)

	transcript := stream.Initial
	lastSentTranscript := ""
	var lastSentAt time.Time

	if initial := strings.TrimSpace(stream.Initial); initial != "" {
		if err := m.replyBus.Reply(ctx, session.chat, session.paneKey, "follow-initial", formatFollowMessage(session.paneKey, initial, m.maxMessageLen)); err != nil {
			return
		}
		m.debugf(session, "initial sent initial_len=%d initial_preview=%s", len([]rune(stream.Initial)), debugPreview(stream.Initial, 180))
		lastSentTranscript = stream.Initial
		lastSentAt = time.Now()
	}

	timer := time.NewTimer(session.minInterval)
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	defer timer.Stop()

	pendingRunes := 0
	dirty := false
	timerActive := false
	chunks := stream.Subscription.Chunks()
	errs := stream.Subscription.Errs()

	stopTimer := func() {
		if !timerActive {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timerActive = false
	}
	scheduleFlush := func(now time.Time) {
		if timerActive {
			return
		}
		wait := session.minInterval
		if !lastSentAt.IsZero() {
			wait = lastSentAt.Add(session.minInterval).Sub(now)
			if wait < 0 {
				wait = 0
			}
		}
		timer.Reset(wait)
		timerActive = true
	}
	flush := func(flushCtx context.Context) bool {
		if !dirty {
			return true
		}
		currentView := buildRecentFollowContext(transcript, 6, 600)
		previousView := buildRecentFollowContext(lastSentTranscript, 6, 600)
		text, changed := buildFollowUpdate(lastSentTranscript, transcript)
		dirty = false
		pendingRunes = 0
		m.debugf(session, "flush changed=%t transcript_len=%d prev_view=%s curr_view=%s send=%s", changed, len([]rune(transcript)), debugPreview(previousView, 160), debugPreview(currentView, 160), debugPreview(text, 180))
		if !changed {
			return true
		}
		if err := m.replyBus.Reply(flushCtx, session.chat, session.paneKey, "follow-output", formatFollowMessage(session.paneKey, text, m.maxMessageLen)); err != nil {
			return false
		}
		lastSentTranscript = transcript
		lastSentAt = time.Now()
		return true
	}
	appendChunk := func(text string) {
		if text == "" {
			return
		}
		transcript = appendFollowTranscript(transcript, text, m.maxMessageLen*8)
		pendingRunes += len([]rune(text))
		dirty = true
		m.debugf(session, "chunk appended chunk_len=%d pending=%d transcript_len=%d chunk_preview=%s", len([]rune(text)), pendingRunes, len([]rune(transcript)), debugPreview(text, 180))
	}
	drainPendingChunks := func() {
		for chunks != nil {
			select {
			case chunk, ok := <-chunks:
				if !ok {
					chunks = nil
					return
				}
				appendChunk(chunk.Text)
			default:
				return
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			drainPendingChunks()
			flushCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = flush(flushCtx)
			cancel()
			return
		case err, ok := <-errs:
			if !ok {
				errs = nil
				if chunks == nil {
					drainPendingChunks()
					_ = flush(ctx)
					return
				}
				continue
			}
			drainPendingChunks()
			if !flush(ctx) {
				return
			}
			_ = m.replyBus.Reply(ctx, session.chat, session.paneKey, "follow-error", fmt.Sprintf("follow stopped for %s: %v", session.paneKey, err))
			return
		case chunk, ok := <-chunks:
			if !ok {
				chunks = nil
				if errs == nil {
					drainPendingChunks()
					_ = flush(ctx)
					return
				}
				continue
			}
			appendChunk(chunk.Text)
			if pendingRunes >= m.maxMessageLen {
				stopTimer()
				if !flush(ctx) {
					return
				}
				continue
			}
			scheduleFlush(time.Now())
		case <-timer.C:
			timerActive = false
			if !flush(ctx) {
				return
			}
		}
	}
}

func (m *FollowManager) normalizeOptions(opts FollowOptions) FollowOptions {
	if opts.MinInterval <= 0 {
		opts.MinInterval = m.minInterval
	}
	return opts
}

func (m *FollowManager) debugf(session *followSession, format string, args ...any) {
	m.mu.Lock()
	w := m.debugWriter
	m.mu.Unlock()
	if w == nil {
		return
	}

	label := "follow-debug"
	if session != nil {
		label = fmt.Sprintf("follow-debug chat=%s pane=%s", session.chat.Key(), session.paneKey)
	}
	message := fmt.Sprintf(format, args...)

	m.debugMu.Lock()
	defer m.debugMu.Unlock()
	fmt.Fprintf(w, "%s %s %s\n", time.Now().Format(time.RFC3339Nano), label, message)
}

const followRepeatedPrefixMarker = "...[omitted repeated prefix]\n"

func buildFollowUpdate(previous string, current string) (string, bool) {
	if strings.TrimSpace(current) == "" {
		return "", false
	}
	if previous == "" {
		return strings.TrimSpace(current), true
	}
	if current == previous {
		return "", false
	}
	if strings.HasPrefix(current, previous) {
		delta := current[len(previous):]
		currentView := buildRecentFollowContext(current, 6, 600)
		previousView := buildRecentFollowContext(previous, 6, 600)
		if shouldPreferFollowContextView(delta, previousView, currentView) {
			return currentView, true
		}
		if shouldSendFollowContext(current, delta) {
			currentView = buildRecentFollowContext(current, 4, 240)
			previousView = buildRecentFollowContext(previous, 4, 240)
			if currentView == "" || currentView == previousView {
				return "", false
			}
			return currentView, true
		}
		text := strings.TrimSpace(delta)
		if text == "" {
			return "", false
		}
		return text, true
	}

	if tail, ok := trimRepeatedPrefix(previous, current, 2, 60); ok {
		return followRepeatedPrefixMarker + tail, true
	}

	currentView := buildRecentFollowContext(current, 6, 600)
	previousView := buildRecentFollowContext(previous, 6, 600)
	if currentView == "" || currentView == previousView {
		return "", false
	}
	return currentView, true
}

func shouldPreferFollowContextView(delta string, previousView string, currentView string) bool {
	delta = strings.TrimSpace(delta)
	currentView = strings.TrimSpace(currentView)
	previousView = strings.TrimSpace(previousView)
	if delta == "" || currentView == "" || currentView == previousView {
		return false
	}
	if !strings.Contains(delta, "\n") {
		return false
	}

	deltaLen := len([]rune(delta))
	viewLen := len([]rune(currentView))
	if deltaLen >= 400 && viewLen <= 220 {
		return true
	}
	if viewLen > 0 && deltaLen >= viewLen*3 {
		return true
	}
	return false
}

func shouldSendFollowContext(current string, delta string) bool {
	if !strings.HasSuffix(current, "\n") {
		return true
	}
	trimmed := strings.TrimSpace(delta)
	if trimmed == "" {
		return false
	}
	return !strings.Contains(delta, "\n") && len([]rune(trimmed)) <= 24
}

func buildRecentFollowContext(text string, maxLines int, maxLen int) string {
	lines := effectiveFollowLines(text)
	if len(lines) == 0 {
		return ""
	}

	selected := make([]string, 0, min(len(lines), maxLines))
	used := 0

	for i := len(lines) - 1; i >= 0 && len(selected) < maxLines; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" && used > 0 {
			continue
		}
		lineLen := len([]rune(line))
		added := lineLen
		if len(selected) > 0 {
			added++
		}
		if used > 0 && used+added > maxLen {
			break
		}
		if used == 0 && lineLen > maxLen {
			return truncateLineTail(line, maxLen)
		}
		selected = append(selected, line)
		used += added
	}

	if len(selected) == 0 {
		return ""
	}
	for left, right := 0, len(selected)-1; left < right; left, right = left+1, right-1 {
		selected[left], selected[right] = selected[right], selected[left]
	}
	return strings.TrimSpace(strings.Join(selected, "\n"))
}

func effectiveFollowLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil
	}

	rawLines := strings.Split(text, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		if idx := strings.LastIndexByte(line, '\r'); idx >= 0 {
			line = line[idx+1:]
		}
		lines = append(lines, line)
	}
	return lines
}

func truncateLineTail(line string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(line)
	if len(runes) <= maxLen {
		return line
	}
	const marker = "... "
	markerRunes := []rune(marker)
	if len(markerRunes) >= maxLen {
		return string(runes[len(runes)-maxLen:])
	}
	tailLen := maxLen - len(markerRunes)
	return marker + string(runes[len(runes)-tailLen:])
}

func debugPreview(text string, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = 120
	}
	quoted := strconv.QuoteToASCII(text)
	runes := []rune(quoted)
	if len(runes) <= maxRunes {
		return quoted
	}
	head := maxRunes / 2
	tail := maxRunes - head - 3
	if tail < 0 {
		tail = 0
	}
	return string(runes[:head]) + "..." + string(runes[len(runes)-tail:])
}

func trimRepeatedPrefix(previous string, current string, minSharedLines int, minSharedRunes int) (string, bool) {
	if strings.HasPrefix(current, previous) {
		tail := strings.TrimLeft(strings.TrimPrefix(current, previous), "\n")
		tail = strings.TrimSpace(tail)
		if tail == "" {
			return "", false
		}
		return tail, true
	}

	prevLines := strings.Split(previous, "\n")
	currLines := strings.Split(current, "\n")
	shared := commonPrefixLines(prevLines, currLines)
	if shared == 0 {
		return "", false
	}
	sharedText := strings.Join(currLines[:shared], "\n")
	if shared < minSharedLines || len([]rune(sharedText)) < minSharedRunes {
		return "", false
	}

	tail := strings.TrimSpace(strings.Join(currLines[shared:], "\n"))
	if tail == "" {
		return "", false
	}
	return tail, true
}

func appendFollowTranscript(current string, chunk string, maxRunes int) string {
	if chunk == "" {
		return current
	}
	current += chunk
	if maxRunes <= 0 {
		return current
	}

	runes := []rune(current)
	if len(runes) <= maxRunes {
		return current
	}

	trimmed := string(runes[len(runes)-maxRunes:])
	if idx := strings.IndexByte(trimmed, '\n'); idx >= 0 && idx+1 < len(trimmed) {
		return trimmed[idx+1:]
	}
	return trimmed
}

func commonPrefixLines(left []string, right []string) int {
	limit := min(len(left), len(right))
	for i := 0; i < limit; i++ {
		if left[i] != right[i] {
			return i
		}
	}
	return limit
}

func (m *FollowManager) removeSession(chatKey string, paneKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session := m.sessions[chatKey]
	if session == nil {
		return
	}
	if session.paneKey != paneKey {
		return
	}
	delete(m.sessions, chatKey)
}

func formatFollowMessage(paneKey string, text string, maxLen int) string {
	text = strings.TrimSpace(text)
	if maxLen > 0 {
		text = truncateForTelegram(text, maxLen)
	}
	if text == "" {
		return fmt.Sprintf("[%s] (empty output)", paneKey)
	}
	return fmt.Sprintf("[%s]\n%s", paneKey, text)
}

func truncateForTelegram(text string, maxLen int) string {
	if maxLen <= 0 {
		return text
	}

	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}

	const marker = "...[truncated]\n"
	markerRunes := []rune(marker)
	if len(markerRunes) >= maxLen {
		return string(runes[len(runes)-maxLen:])
	}

	tailLen := maxLen - len(markerRunes)
	if tail := trailingLinesWithinLimit(text, tailLen); tail != "" {
		return marker + tail
	}

	tail := string(runes[len(runes)-tailLen:])
	return marker + tail
}

func trailingLinesWithinLimit(text string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}

	lines := strings.Split(text, "\n")
	selected := make([]string, 0, len(lines))
	used := 0

	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		lineLen := len([]rune(line))
		added := lineLen
		if len(selected) > 0 {
			added++
		}
		if added > maxLen-used {
			break
		}
		selected = append(selected, line)
		used += added
	}

	if len(selected) == 0 {
		return ""
	}

	for left, right := 0, len(selected)-1; left < right; left, right = left+1, right-1 {
		selected[left], selected[right] = selected[right], selected[left]
	}
	return strings.Join(selected, "\n")
}
