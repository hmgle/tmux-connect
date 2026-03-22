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
			if !session.shouldFlushOnCancel() {
				return
			}
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
