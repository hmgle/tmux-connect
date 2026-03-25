package daemon

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"
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
	session := newFollowSession(chat, stream.Pane.Target.PaneKey(), opts.MinInterval, cancel)
	chatKey := chat.Key()

	go m.runSession(runCtx, session, stream)
	session.waitUntilStarted()

	if existing := m.swapSession(chatKey, session); existing != nil {
		existing.stop(false)
		existing.wait()
	}
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
	session := m.detachSession(chatKey)
	if session == nil {
		return false
	}
	session.stop(true)
	session.wait()
	return true
}

func (m *FollowManager) StopPane(paneKey string) {
	paneKey = strings.TrimSpace(paneKey)
	sessions := m.detachPaneSessions(paneKey)
	for _, session := range sessions {
		session.stop(true)
		session.wait()
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
	for _, session := range m.detachAllSessions() {
		session.stop(true)
		session.wait()
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

func (m *FollowManager) warnf(session *followSession, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if session != nil {
		log.Printf("warn: follow chat=%s pane=%s %s", session.chat.Key(), session.paneKey, message)
	} else {
		log.Printf("warn: follow %s", message)
	}
	m.debugf(session, "%s", message)
}
