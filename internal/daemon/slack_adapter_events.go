//go:build !no_slack

package daemon

import (
	"context"
	"strings"

	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

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
		text = "help"
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
		IsAppMention: true,
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

func stripSlackAppMentionText(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "<@") {
		if idx := strings.Index(text, ">"); idx != -1 {
			return strings.TrimSpace(text[idx+1:])
		}
	}
	return text
}
