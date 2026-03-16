package daemon

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/portgle/tmux-connect/internal/tagb"
)

type FollowManager struct {
	service       paneService
	replyBus      *ReplyBus
	initialLines  int
	minInterval   time.Duration
	maxMessageLen int

	mu       sync.Mutex
	sessions map[int64]*followSession
}

type FollowOptions struct {
	MinInterval time.Duration
}

type followSession struct {
	chatID      int64
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
		sessions:      make(map[int64]*followSession),
	}
}

func (m *FollowManager) Enable(ctx context.Context, chatID int64, paneKey string) error {
	return m.EnableWithOptions(ctx, chatID, paneKey, FollowOptions{})
}

func (m *FollowManager) EnableWithOptions(ctx context.Context, chatID int64, paneKey string, opts FollowOptions) error {
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
		chatID:      chatID,
		paneKey:     stream.Pane.Target.PaneKey(),
		minInterval: opts.MinInterval,
		cancel:      cancel,
	}

	m.mu.Lock()
	if existing := m.sessions[chatID]; existing != nil {
		existing.cancel()
	}
	m.sessions[chatID] = session
	m.mu.Unlock()

	go m.run(runCtx, session, stream)
	return nil
}

func (m *FollowManager) Options(chatID int64) FollowOptions {
	m.mu.Lock()
	defer m.mu.Unlock()

	session := m.sessions[chatID]
	if session == nil {
		return FollowOptions{MinInterval: m.minInterval}
	}
	return FollowOptions{MinInterval: session.minInterval}
}

func (m *FollowManager) Disable(chatID int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	session := m.sessions[chatID]
	if session == nil {
		return false
	}
	delete(m.sessions, chatID)
	session.cancel()
	return true
}

func (m *FollowManager) StopPane(paneKey string) {
	paneKey = strings.TrimSpace(paneKey)
	m.mu.Lock()
	defer m.mu.Unlock()
	for chatID, session := range m.sessions {
		if session.paneKey != paneKey {
			continue
		}
		delete(m.sessions, chatID)
		session.cancel()
	}
}

func (m *FollowManager) IsEnabled(chatID int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[chatID] != nil
}

func (m *FollowManager) CurrentPane(chatID int64) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if session := m.sessions[chatID]; session != nil {
		return session.paneKey
	}
	return ""
}

func (m *FollowManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for chatID, session := range m.sessions {
		delete(m.sessions, chatID)
		session.cancel()
	}
}

func (m *FollowManager) run(ctx context.Context, session *followSession, stream tagb.PaneStream) {
	defer stream.Subscription.Close()
	defer m.removeSession(session.chatID, session.paneKey)

	lastSentText := ""
	var lastSentAt time.Time

	if initial := strings.TrimSpace(stream.Initial); initial != "" {
		if err := m.replyBus.Reply(ctx, session.chatID, session.paneKey, "follow-initial", formatFollowMessage(session.paneKey, initial, m.maxMessageLen)); err != nil {
			return
		}
		lastSentText = initial
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

	var builder strings.Builder
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
		raw := strings.TrimSpace(builder.String())
		builder.Reset()
		if raw == "" {
			return true
		}
		text, changed := prepareFollowMessageDelta(lastSentText, raw)
		if !changed {
			return true
		}
		if err := m.replyBus.Reply(flushCtx, session.chatID, session.paneKey, "follow-output", formatFollowMessage(session.paneKey, text, m.maxMessageLen)); err != nil {
			return false
		}
		lastSentText = raw
		lastSentAt = time.Now()
		return true
	}
	drainPendingChunks := func() {
		for chunks != nil {
			select {
			case chunk, ok := <-chunks:
				if !ok {
					chunks = nil
					return
				}
				builder.WriteString(chunk.Text)
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
			_ = m.replyBus.Reply(ctx, session.chatID, session.paneKey, "follow-error", fmt.Sprintf("follow stopped for %s: %v", session.paneKey, err))
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
			builder.WriteString(chunk.Text)
			if builder.Len() >= m.maxMessageLen {
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

const followRepeatedPrefixMarker = "...[omitted repeated prefix]\n"

func prepareFollowMessageDelta(previous string, current string) (string, bool) {
	previous = strings.TrimSpace(previous)
	current = strings.TrimSpace(current)
	if current == "" {
		return "", false
	}
	if previous == "" {
		return current, true
	}
	if current == previous {
		return "", false
	}
	if tail, ok := trimRepeatedPrefix(previous, current); ok {
		return followRepeatedPrefixMarker + tail, true
	}
	return current, true
}

func trimRepeatedPrefix(previous string, current string) (string, bool) {
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
	if shared < 2 || len([]rune(sharedText)) < 60 {
		return "", false
	}

	tail := strings.TrimSpace(strings.Join(currLines[shared:], "\n"))
	if tail == "" {
		return "", false
	}
	return tail, true
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

func (m *FollowManager) removeSession(chatID int64, paneKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session := m.sessions[chatID]
	if session == nil {
		return
	}
	if session.paneKey != paneKey {
		return
	}
	delete(m.sessions, chatID)
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
