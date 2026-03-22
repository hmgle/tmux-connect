package daemon

import (
	"context"
	"sync"
	"time"
)

type followSession struct {
	chat        ChatRef
	paneKey     string
	minInterval time.Duration
	cancel      context.CancelFunc
	started     chan struct{}
	done        chan struct{}

	stopMu        sync.Mutex
	stopRequested bool
	flushOnCancel bool
}

func newFollowSession(chat ChatRef, paneKey string, minInterval time.Duration, cancel context.CancelFunc) *followSession {
	return &followSession{
		chat:          chat,
		paneKey:       paneKey,
		minInterval:   minInterval,
		cancel:        cancel,
		started:       make(chan struct{}),
		done:          make(chan struct{}),
		flushOnCancel: true,
	}
}

func (s *followSession) markStarted() {
	close(s.started)
}

func (s *followSession) markDone() {
	close(s.done)
}

func (s *followSession) waitUntilStarted() {
	<-s.started
}

func (s *followSession) wait() {
	<-s.done
}

func (s *followSession) stop(flush bool) {
	s.stopMu.Lock()
	if !s.stopRequested {
		s.flushOnCancel = flush
		s.stopRequested = true
		cancel := s.cancel
		s.stopMu.Unlock()
		cancel()
		return
	}
	if flush {
		s.flushOnCancel = true
	}
	s.stopMu.Unlock()
}

func (s *followSession) shouldFlushOnCancel() bool {
	s.stopMu.Lock()
	defer s.stopMu.Unlock()
	return s.flushOnCancel
}

func (m *FollowManager) swapSession(chatKey string, session *followSession) *followSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	previous := m.sessions[chatKey]
	m.sessions[chatKey] = session
	return previous
}

func (m *FollowManager) detachSession(chatKey string) *followSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	session := m.sessions[chatKey]
	if session != nil {
		delete(m.sessions, chatKey)
	}
	return session
}

func (m *FollowManager) detachPaneSessions(paneKey string) []*followSession {
	m.mu.Lock()
	defer m.mu.Unlock()

	var sessions []*followSession
	for chatKey, session := range m.sessions {
		if session.paneKey != paneKey {
			continue
		}
		delete(m.sessions, chatKey)
		sessions = append(sessions, session)
	}
	return sessions
}

func (m *FollowManager) detachAllSessions() []*followSession {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessions := make([]*followSession, 0, len(m.sessions))
	for chatKey, session := range m.sessions {
		delete(m.sessions, chatKey)
		sessions = append(sessions, session)
	}
	return sessions
}

func (m *FollowManager) removeSession(chatKey string, session *followSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current := m.sessions[chatKey]
	if current == nil {
		return
	}
	if current != session {
		return
	}
	delete(m.sessions, chatKey)
}
