package daemon

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	slackclient "github.com/hmgle/tmux-connect/internal/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type slackAdapter struct {
	client *slackclient.Client
	stderr io.Writer
	store  *Store

	mu            sync.Mutex
	activeThreads map[string]time.Time
	threadTTL     time.Duration
	maxThreads    int
	now           func() time.Time
}

const (
	defaultSlackThreadTTL  = 24 * time.Hour
	defaultSlackMaxThreads = 2048
)

func newSlackAdapter(cfg Config, stderr io.Writer, store *Store) (platformAdapter, error) {
	if strings.TrimSpace(cfg.SlackBotToken) == "" {
		return nil, fmt.Errorf("slack bot token is required")
	}
	if strings.TrimSpace(cfg.SlackAppToken) == "" {
		return nil, fmt.Errorf("slack app token is required")
	}
	return &slackAdapter{
		client:        slackclient.NewClient(cfg.SlackBotToken, cfg.SlackAppToken),
		stderr:        stderr,
		store:         store,
		activeThreads: make(map[string]time.Time),
		threadTTL:     defaultSlackThreadTTL,
		maxThreads:    defaultSlackMaxThreads,
		now:           time.Now,
	}, nil
}

func (a *slackAdapter) Platform() string { return "slack" }

func (a *slackAdapter) SendMessage(ctx context.Context, chat ChatRef, text string, opts SendOptions) (OutboundMessage, error) {
	threadID := strings.TrimSpace(opts.ThreadID)
	if threadID == "" {
		threadID = strings.TrimSpace(opts.ReplyToMessageID)
	}
	ts, err := a.client.PostMessage(ctx, chat.ChatID, text, threadID)
	if err != nil {
		return OutboundMessage{}, err
	}
	if threadID != "" {
		a.rememberThread(chat.ChatID, threadID)
	}
	return OutboundMessage{MessageID: ts}, nil
}

func (a *slackAdapter) SendImage(ctx context.Context, chat ChatRef, fileName string, data []byte, caption string, opts SendOptions) (OutboundMessage, error) {
	threadID := strings.TrimSpace(opts.ThreadID)
	if threadID == "" {
		threadID = strings.TrimSpace(opts.ReplyToMessageID)
	}
	id, err := a.client.UploadImage(ctx, chat.ChatID, threadID, fileName, data, caption)
	if err != nil {
		return OutboundMessage{}, err
	}
	if threadID != "" {
		a.rememberThread(chat.ChatID, threadID)
	}
	return OutboundMessage{MessageID: id}, nil
}

func (a *slackAdapter) DecorateMessage(kind string, text string, opts SendOptions) (string, SendOptions) {
	return decorateSlackMessage(kind, text, opts)
}

func (a *slackAdapter) PromptOptions(message IncomingMessage, _ commandPromptSpec) SendOptions {
	return SendOptions{ThreadID: message.replyThreadID()}
}

func (a *slackAdapter) SnapshotCaption(paneKey string) string {
	return formatSnapshotCaption(paneKey)
}

func (a *slackAdapter) Run(ctx context.Context, handler func(context.Context, IncomingMessage) error) error {
	go func() {
		if err := a.client.Run(ctx); err != nil && ctx.Err() == nil {
			fmt.Fprintf(a.stderr, "slack socket mode error: %v\n", err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-a.client.Events():
			if !ok {
				return nil
			}
			if err := a.handleEvent(ctx, evt, handler); err != nil {
				fmt.Fprintf(a.stderr, "slack event error: %v\n", err)
			}
		}
	}
}

func (a *slackAdapter) RegisterCommands(context.Context, []botCommandSpec) error {
	return nil
}

func (a *slackAdapter) Close() error { return nil }

func (a *slackAdapter) handleEvent(ctx context.Context, evt socketmode.Event, handler func(context.Context, IncomingMessage) error) error {
	switch evt.Type {
	case socketmode.EventTypeEventsAPI:
		data, ok := evt.Data.(slackevents.EventsAPIEvent)
		if !ok {
			return nil
		}
		if evt.Request != nil {
			a.client.Ack(*evt.Request)
		}
		if data.Type != slackevents.CallbackEvent {
			return nil
		}
		switch ev := data.InnerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:
			return a.handleAppMention(ctx, ev, handler)
		case *slackevents.MessageEvent:
			return a.handleMessage(ctx, ev, handler)
		}
	}
	return nil
}

func (a *slackAdapter) handleAppMention(ctx context.Context, ev *slackevents.AppMentionEvent, handler func(context.Context, IncomingMessage) error) error {
	if ev == nil || ev.BotID != "" || ev.User == "" || isOldSlackTimestamp(ev.TimeStamp) {
		return nil
	}
	text := stripSlackAppMentionText(ev.Text)
	if text == "" {
		return nil
	}
	threadID := strings.TrimSpace(ev.ThreadTimeStamp)
	if threadID == "" {
		threadID = strings.TrimSpace(ev.TimeStamp)
	}
	a.rememberThread(ev.Channel, threadID)
	return handler(ctx, IncomingMessage{
		Chat: ChatRef{
			Platform: a.Platform(),
			ChatID:   strings.TrimSpace(ev.Channel),
		},
		MessageID:    strings.TrimSpace(ev.TimeStamp),
		UserID:       strings.TrimSpace(ev.User),
		Text:         text,
		ThreadID:     threadID,
		PendingScope: threadID,
	})
}

func (a *slackAdapter) handleMessage(ctx context.Context, ev *slackevents.MessageEvent, handler func(context.Context, IncomingMessage) error) error {
	if ev == nil || ev.BotID != "" || ev.User == "" || isOldSlackTimestamp(ev.TimeStamp) {
		return nil
	}
	threadID := strings.TrimSpace(ev.ThreadTimeStamp)
	isDM := strings.TrimSpace(ev.ChannelType) == "im"
	if !isDM {
		if threadID == "" || !a.isKnownThread(ctx, ev.Channel, threadID) {
			return nil
		}
	}
	if strings.TrimSpace(ev.Text) == "" {
		return nil
	}
	if threadID == "" {
		threadID = strings.TrimSpace(ev.TimeStamp)
	}
	a.rememberThread(ev.Channel, threadID)
	return handler(ctx, IncomingMessage{
		Chat: ChatRef{
			Platform: a.Platform(),
			ChatID:   strings.TrimSpace(ev.Channel),
		},
		MessageID:    strings.TrimSpace(ev.TimeStamp),
		UserID:       strings.TrimSpace(ev.User),
		Text:         strings.TrimSpace(ev.Text),
		ThreadID:     threadID,
		PendingScope: threadID,
	})
}

func (a *slackAdapter) rememberThread(channel string, threadID string) {
	channel = strings.TrimSpace(channel)
	threadID = strings.TrimSpace(threadID)
	if channel == "" || threadID == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.timeNow()
	a.pruneExpiredLocked(now)
	a.activeThreads[channel+"|"+threadID] = now
	a.evictOverflowLocked()
}

func (a *slackAdapter) isActiveThread(channel string, threadID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.timeNow()
	a.pruneExpiredLocked(now)
	_, ok := a.activeThreads[strings.TrimSpace(channel)+"|"+strings.TrimSpace(threadID)]
	return ok
}

func (a *slackAdapter) isKnownThread(ctx context.Context, channel string, threadID string) bool {
	if a.isActiveThread(channel, threadID) {
		return true
	}
	if !a.hasPersistedThread(ctx, channel, threadID) {
		return false
	}
	a.rememberThread(channel, threadID)
	return true
}

func (a *slackAdapter) hasPersistedThread(ctx context.Context, channel string, threadID string) bool {
	if a.store == nil {
		return false
	}
	ok, err := a.store.HasThread(ctx, ChatRef{
		Platform: a.Platform(),
		ChatID:   strings.TrimSpace(channel),
	}, threadID)
	if err != nil {
		if a.stderr != nil {
			fmt.Fprintf(a.stderr, "slack thread lookup error: %v\n", err)
		}
		return false
	}
	return ok
}

func (a *slackAdapter) timeNow() time.Time {
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}

func (a *slackAdapter) pruneExpiredLocked(now time.Time) {
	if a.threadTTL <= 0 {
		return
	}
	cutoff := now.Add(-a.threadTTL)
	for key, lastSeen := range a.activeThreads {
		if lastSeen.Before(cutoff) {
			delete(a.activeThreads, key)
		}
	}
}

func (a *slackAdapter) evictOverflowLocked() {
	if a.maxThreads <= 0 {
		return
	}
	for len(a.activeThreads) > a.maxThreads {
		oldestKey := ""
		var oldestSeen time.Time
		for key, lastSeen := range a.activeThreads {
			if oldestKey == "" || lastSeen.Before(oldestSeen) {
				oldestKey = key
				oldestSeen = lastSeen
			}
		}
		if oldestKey == "" {
			return
		}
		delete(a.activeThreads, oldestKey)
	}
}

func (m IncomingMessage) replyThreadID() string {
	if strings.TrimSpace(m.ThreadID) != "" {
		return strings.TrimSpace(m.ThreadID)
	}
	return strings.TrimSpace(m.MessageID)
}

func stripSlackAppMentionText(text string) string {
	text = strings.TrimSpace(text)
	if idx := strings.Index(text, "> "); idx != -1 && strings.HasPrefix(text, "<@") {
		return strings.TrimSpace(text[idx+2:])
	}
	return text
}

func isOldSlackTimestamp(ts string) bool {
	ts = strings.TrimSpace(ts)
	if ts == "" {
		return false
	}
	dot := strings.IndexByte(ts, '.')
	if dot <= 0 {
		return false
	}
	sec, err := strconv.ParseInt(ts[:dot], 10, 64)
	if err != nil {
		return false
	}
	return time.Since(time.Unix(sec, 0)) > 2*time.Minute
}
