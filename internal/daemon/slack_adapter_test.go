package daemon

import (
	"context"
	"testing"

	"github.com/slack-go/slack/slackevents"
)

func TestSlackHandleAppMentionBuildsThreadScopedMessage(t *testing.T) {
	t.Parallel()

	adapter := &slackAdapter{activeThread: make(map[string]struct{})}
	var got IncomingMessage
	err := adapter.handleAppMention(context.Background(), &slackevents.AppMentionEvent{
		User:      "U123",
		Channel:   "C123",
		TimeStamp: "1893456000.000100",
		Text:      "<@U0BOT123> /panes",
	}, func(_ context.Context, message IncomingMessage) error {
		got = message
		return nil
	})
	if err != nil {
		t.Fatalf("handleAppMention() error = %v", err)
	}
	if got.Chat.Platform != "slack" || got.Chat.ChatID != "C123" {
		t.Fatalf("chat = %#v, want slack/C123", got.Chat)
	}
	if got.Text != "/panes" {
		t.Fatalf("text = %q, want /panes", got.Text)
	}
	if got.ThreadID != "1893456000.000100" {
		t.Fatalf("threadID = %q, want root timestamp", got.ThreadID)
	}
	if got.PendingScope != "1893456000.000100" {
		t.Fatalf("pending scope = %q, want root timestamp", got.PendingScope)
	}
}

func TestSlackHandleMessageAcceptsDM(t *testing.T) {
	t.Parallel()

	adapter := &slackAdapter{activeThread: make(map[string]struct{})}
	var got IncomingMessage
	err := adapter.handleMessage(context.Background(), &slackevents.MessageEvent{
		User:        "U123",
		Channel:     "D123",
		ChannelType: "im",
		TimeStamp:   "1893456000.000200",
		Text:        "/current",
	}, func(_ context.Context, message IncomingMessage) error {
		got = message
		return nil
	})
	if err != nil {
		t.Fatalf("handleMessage() error = %v", err)
	}
	if got.Chat.ChatID != "D123" {
		t.Fatalf("chat = %#v, want D123", got.Chat)
	}
	if got.ThreadID != "1893456000.000200" {
		t.Fatalf("threadID = %q, want message timestamp", got.ThreadID)
	}
}

func TestSlackHandleMessageRejectsUnknownChannelThread(t *testing.T) {
	t.Parallel()

	adapter := &slackAdapter{activeThread: make(map[string]struct{})}
	called := false
	err := adapter.handleMessage(context.Background(), &slackevents.MessageEvent{
		User:            "U123",
		Channel:         "C123",
		ChannelType:     "channel",
		TimeStamp:       "1893456000.000300",
		ThreadTimeStamp: "1893456000.000250",
		Text:            "follow up",
	}, func(_ context.Context, message IncomingMessage) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("handleMessage() error = %v", err)
	}
	if called {
		t.Fatal("handleMessage() unexpectedly accepted unknown thread")
	}
}

func TestSlackHandleMessageAcceptsKnownChannelThread(t *testing.T) {
	t.Parallel()

	adapter := &slackAdapter{activeThread: make(map[string]struct{})}
	adapter.rememberThread("C123", "1893456000.000400")

	var got IncomingMessage
	err := adapter.handleMessage(context.Background(), &slackevents.MessageEvent{
		User:            "U123",
		Channel:         "C123",
		ChannelType:     "channel",
		TimeStamp:       "1893456000.000401",
		ThreadTimeStamp: "1893456000.000400",
		Text:            "/snapshot",
	}, func(_ context.Context, message IncomingMessage) error {
		got = message
		return nil
	})
	if err != nil {
		t.Fatalf("handleMessage() error = %v", err)
	}
	if got.ThreadID != "1893456000.000400" {
		t.Fatalf("threadID = %q, want remembered thread root", got.ThreadID)
	}
	if got.PendingScope != "1893456000.000400" {
		t.Fatalf("pending scope = %q, want thread root", got.PendingScope)
	}
}

func TestDecorateSlackMessageUsesCodeBlocksForSnapshot(t *testing.T) {
	t.Parallel()

	text, opts := decorateSlackMessage("snapshot", "[default:%5]\nhello", SendOptions{})
	if text != "```[default:%5]\nhello```" {
		t.Fatalf("text = %q, want fenced code block", text)
	}
	if opts != (SendOptions{}) {
		t.Fatalf("opts changed = %#v, want unchanged", opts)
	}
}
