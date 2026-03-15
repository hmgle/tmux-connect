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
	flushInterval time.Duration
	maxMessageLen int

	mu       sync.Mutex
	sessions map[int64]*followSession
}

type followSession struct {
	chatID  int64
	paneKey string
	cancel  context.CancelFunc
}

func NewFollowManager(service paneService, replyBus *ReplyBus, initialLines int) *FollowManager {
	return &FollowManager{
		service:       service,
		replyBus:      replyBus,
		initialLines:  initialLines,
		flushInterval: 700 * time.Millisecond,
		maxMessageLen: 3500,
		sessions:      make(map[int64]*followSession),
	}
}

func (m *FollowManager) Enable(ctx context.Context, chatID int64, paneKey string) error {
	paneKey = strings.TrimSpace(paneKey)
	if paneKey == "" {
		return fmt.Errorf("pane key is required")
	}

	stream, err := m.service.OpenStream(ctx, paneKey, m.initialLines)
	if err != nil {
		return err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	session := &followSession{
		chatID:  chatID,
		paneKey: stream.Pane.Target.PaneKey(),
		cancel:  cancel,
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

	if initial := strings.TrimSpace(stream.Initial); initial != "" {
		if err := m.replyBus.Reply(ctx, session.chatID, session.paneKey, "follow-initial", formatFollowMessage(session.paneKey, initial, m.maxMessageLen)); err != nil {
			return
		}
	}

	timer := time.NewTimer(m.flushInterval)
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

	flush := func(flushCtx context.Context) bool {
		text := strings.TrimSpace(builder.String())
		builder.Reset()
		if text == "" {
			return true
		}
		if err := m.replyBus.Reply(flushCtx, session.chatID, session.paneKey, "follow-output", formatFollowMessage(session.paneKey, text, m.maxMessageLen)); err != nil {
			return false
		}
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
				if !flush(ctx) {
					return
				}
				timerActive = false
				continue
			}
			if timerActive && !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(m.flushInterval)
			timerActive = true
		case <-timer.C:
			timerActive = false
			if !flush(ctx) {
				return
			}
		}
	}
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
		runes := []rune(text)
		if len(runes) > maxLen {
			text = string(runes[:maxLen]) + "\n...[truncated]"
		}
	}
	if text == "" {
		return fmt.Sprintf("[%s] (empty output)", paneKey)
	}
	return fmt.Sprintf("[%s]\n%s", paneKey, text)
}
