package daemon

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

func (m *FollowManager) runSession(ctx context.Context, session *followSession, stream tmuxconn.PaneStream) {
	session.markStarted()
	defer session.markDone()
	m.run(ctx, session, stream)
}

func (m *FollowManager) run(ctx context.Context, session *followSession, stream tmuxconn.PaneStream) {
	defer stream.Subscription.Close()
	defer m.removeSession(session.chat.Key(), session)

	transcript := stream.Initial
	lastSentTranscript := ""
	var lastSentAt time.Time

	timer := time.NewTimer(session.minInterval)
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	defer timer.Stop()

	pendingRunes := 0
	dirty := strings.TrimSpace(stream.Initial) != ""
	timerActive := false
	sendFailures := 0
	var lastSendErr error
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
	scheduleRetry := func() {
		if timerActive {
			return
		}
		wait := session.minInterval
		if wait <= 0 {
			wait = 700 * time.Millisecond
		}
		for attempt := 1; attempt < sendFailures && wait < 30*time.Second; attempt++ {
			wait *= 2
			if wait > 30*time.Second {
				wait = 30 * time.Second
			}
		}
		timer.Reset(wait)
		timerActive = true
	}
	flush := func(flushCtx context.Context, kind string) bool {
		if !dirty {
			return true
		}
		currentView := buildRecentFollowContext(transcript, 6, 600)
		previousView := buildRecentFollowContext(lastSentTranscript, 6, 600)
		text, changed := buildFollowUpdate(lastSentTranscript, transcript)
		m.debugf(session, "flush changed=%t transcript_len=%d prev_view=%s curr_view=%s send=%s", changed, len([]rune(transcript)), debugPreview(previousView, 160), debugPreview(currentView, 160), debugPreview(text, 180))
		if !changed {
			dirty = false
			pendingRunes = 0
			return true
		}
		messageKind := kind
		if messageKind == "" {
			messageKind = "follow-output"
		}
		if sendFailures > 0 {
			recoveryNotice := fmt.Sprintf("[follow delivery resumed after %d failed attempt(s)]\n", sendFailures)
			text = recoveryNotice + text
		}
		if err := m.replyBus.Reply(flushCtx, session.chat, session.paneKey, messageKind, formatFollowMessage(session.paneKey, text, m.maxMessageLen)); err != nil {
			lastSendErr = err
			sendFailures++
			pendingRunes = 0
			dirty = true
			m.warnf(session, "delivery failure attempt=%d kind=%s err=%v", sendFailures, messageKind, err)
			return false
		}
		if sendFailures > 0 {
			m.warnf(session, "delivery recovered after %d failed attempt(s); last_err=%v", sendFailures, lastSendErr)
			sendFailures = 0
			lastSendErr = nil
		}
		dirty = false
		pendingRunes = 0
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

	if dirty && !flush(ctx, "follow-initial") {
		scheduleRetry()
	}

	for {
		select {
		case <-ctx.Done():
			if !session.shouldFlushOnCancel() {
				return
			}
			drainPendingChunks()
			flushCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			if !flush(flushCtx, "follow-output") {
				m.warnf(session, "dropping buffered follow output on shutdown after delivery failure: %v", lastSendErr)
			}
			cancel()
			return
		case err, ok := <-errs:
			if !ok {
				errs = nil
				if chunks == nil {
					drainPendingChunks()
					if !flush(ctx, "follow-output") {
						m.warnf(session, "stream closed with unsent follow output: %v", lastSendErr)
					}
					return
				}
				continue
			}
			drainPendingChunks()
			if !flush(ctx, "follow-output") {
				m.warnf(session, "follow stream failed while buffered output was unsent: stream_err=%v send_err=%v", err, lastSendErr)
			}
			if replyErr := m.replyBus.Reply(ctx, session.chat, session.paneKey, "follow-error", fmt.Sprintf("follow stopped for %s: %v", session.paneKey, err)); replyErr != nil {
				m.warnf(session, "failed to deliver follow stop notification: %v", replyErr)
			}
			return
		case chunk, ok := <-chunks:
			if !ok {
				chunks = nil
				if errs == nil {
					drainPendingChunks()
					if !flush(ctx, "follow-output") {
						m.warnf(session, "chunk stream closed with unsent follow output: %v", lastSendErr)
					}
					return
				}
				continue
			}
			appendChunk(chunk.Text)
			if pendingRunes >= m.maxMessageLen {
				stopTimer()
				if !flush(ctx, "follow-output") {
					scheduleRetry()
				}
				continue
			}
			scheduleFlush(time.Now())
		case <-timer.C:
			timerActive = false
			if !flush(ctx, "follow-output") {
				scheduleRetry()
			}
		}
	}
}
