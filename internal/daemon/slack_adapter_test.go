package daemon

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/slack-go/slack/slackevents"
)

func newTestSlackAdapter(store *Store) *slackAdapter {
	return &slackAdapter{
		store:         store,
		activeThreads: make(map[string]time.Time),
		threadTTL:     defaultSlackThreadTTL,
		maxThreads:    defaultSlackMaxThreads,
		now:           time.Now,
	}
}

func TestSlackHandleAppMentionBuildsThreadScopedMessage(t *testing.T) {
	t.Parallel()

	adapter := newTestSlackAdapter(nil)
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

	adapter := newTestSlackAdapter(nil)
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

	adapter := newTestSlackAdapter(nil)
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

	adapter := newTestSlackAdapter(nil)
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

func TestSlackHandleMessageAcceptsPersistedChannelThreadWithoutWarmCache(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	threadID := "1893456000.000400"
	if err := store.LogMessage(ctx, MessageRecord{
		Chat:              ChatRef{Platform: "slack", ChatID: "C123"},
		Direction:         "out",
		Kind:              "reply",
		PlatformMessageID: "1893456000.000401",
		ThreadID:          threadID,
		BodyPreview:       "ready",
	}); err != nil {
		t.Fatalf("LogMessage() error = %v", err)
	}

	adapter := newTestSlackAdapter(store)
	var got IncomingMessage
	err = adapter.handleMessage(ctx, &slackevents.MessageEvent{
		User:            "U123",
		Channel:         "C123",
		ChannelType:     "channel",
		TimeStamp:       "1893456000.000402",
		ThreadTimeStamp: threadID,
		Text:            "/snapshot",
	}, func(_ context.Context, message IncomingMessage) error {
		got = message
		return nil
	})
	if err != nil {
		t.Fatalf("handleMessage() error = %v", err)
	}
	if got.ThreadID != threadID {
		t.Fatalf("threadID = %q, want %q", got.ThreadID, threadID)
	}
	if !adapter.isActiveThread("C123", threadID) {
		t.Fatal("persisted thread was not repopulated into cache")
	}
}

func TestSlackHandleMessageAcceptsPersistedChannelThreadAfterCacheEviction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	threadID := "t1"
	if err := store.LogMessage(ctx, MessageRecord{
		Chat:              ChatRef{Platform: "slack", ChatID: "C123"},
		Direction:         "out",
		Kind:              "reply",
		PlatformMessageID: "m1",
		ThreadID:          threadID,
		BodyPreview:       "ready",
	}); err != nil {
		t.Fatalf("LogMessage() error = %v", err)
	}

	base := time.Unix(1_700_000_000, 0)
	now := base
	adapter := &slackAdapter{
		store:         store,
		activeThreads: make(map[string]time.Time),
		threadTTL:     time.Hour,
		maxThreads:    1,
		now: func() time.Time {
			return now
		},
	}

	adapter.rememberThread("C123", threadID)
	now = now.Add(time.Minute)
	adapter.rememberThread("C123", "t2")
	if adapter.isActiveThread("C123", threadID) {
		t.Fatal("thread should have been evicted from cache")
	}

	now = now.Add(time.Minute)
	called := false
	err = adapter.handleMessage(ctx, &slackevents.MessageEvent{
		User:            "U123",
		Channel:         "C123",
		ChannelType:     "channel",
		TimeStamp:       "m2",
		ThreadTimeStamp: threadID,
		Text:            "/snapshot",
	}, func(_ context.Context, message IncomingMessage) error {
		called = message.ThreadID == threadID
		return nil
	})
	if err != nil {
		t.Fatalf("handleMessage() error = %v", err)
	}
	if !called {
		t.Fatal("handleMessage() should accept persisted thread after cache eviction")
	}
	if !adapter.isActiveThread("C123", threadID) {
		t.Fatal("persisted thread should be re-cached after fallback lookup")
	}
}

func TestSlackRememberThreadPrunesExpiredEntries(t *testing.T) {
	t.Parallel()

	base := time.Unix(1_700_000_000, 0)
	now := base
	adapter := &slackAdapter{
		activeThreads: make(map[string]time.Time),
		threadTTL:     10 * time.Minute,
		maxThreads:    defaultSlackMaxThreads,
		now: func() time.Time {
			return now
		},
	}

	adapter.rememberThread("C1", "t-old")
	now = now.Add(11 * time.Minute)
	adapter.rememberThread("C1", "t-new")

	if adapter.isActiveThread("C1", "t-old") {
		t.Fatal("expired thread still marked active")
	}
	if !adapter.isActiveThread("C1", "t-new") {
		t.Fatal("fresh thread not marked active")
	}
}

func TestSlackRememberThreadEvictsOldestWhenOverCapacity(t *testing.T) {
	t.Parallel()

	base := time.Unix(1_700_000_000, 0)
	now := base
	adapter := &slackAdapter{
		activeThreads: make(map[string]time.Time),
		threadTTL:     time.Hour,
		maxThreads:    2,
		now: func() time.Time {
			return now
		},
	}

	adapter.rememberThread("C1", "t1")
	now = now.Add(time.Minute)
	adapter.rememberThread("C1", "t2")
	now = now.Add(time.Minute)
	adapter.rememberThread("C1", "t3")

	if adapter.isActiveThread("C1", "t1") {
		t.Fatal("oldest thread still present after capacity eviction")
	}
	if !adapter.isActiveThread("C1", "t2") || !adapter.isActiveThread("C1", "t3") {
		t.Fatal("newer threads should remain active after capacity eviction")
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
