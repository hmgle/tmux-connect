package daemon

import (
	"context"
	"path/filepath"
	"testing"
)

func TestStoreBindAndCurrentPane(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	if err := store.BindPane(ctx, 101, "default:%5"); err != nil {
		t.Fatalf("BindPane() error = %v", err)
	}
	if err := store.BindPane(ctx, 101, "default:%8"); err != nil {
		t.Fatalf("BindPane() second error = %v", err)
	}
	if err := store.SetCurrentPane(ctx, 101, "default:%8"); err != nil {
		t.Fatalf("SetCurrentPane() error = %v", err)
	}

	bindings, err := store.ListBindings(ctx, 101)
	if err != nil {
		t.Fatalf("ListBindings() error = %v", err)
	}
	if len(bindings) != 2 {
		t.Fatalf("bindings len = %d, want 2", len(bindings))
	}
	current, err := store.CurrentPane(ctx, 101)
	if err != nil {
		t.Fatalf("CurrentPane() error = %v", err)
	}
	if current != "default:%8" {
		t.Fatalf("current = %q, want %q", current, "default:%8")
	}
}

func TestStoreUnbindPaneEverywhere(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	if err := store.BindPane(ctx, 1, "default:%5"); err != nil {
		t.Fatalf("BindPane() error = %v", err)
	}
	if err := store.SetCurrentPane(ctx, 1, "default:%5"); err != nil {
		t.Fatalf("SetCurrentPane() error = %v", err)
	}
	if err := store.BindPane(ctx, 2, "default:%5"); err != nil {
		t.Fatalf("BindPane() second error = %v", err)
	}

	if err := store.UnbindPaneEverywhere(ctx, "default:%5"); err != nil {
		t.Fatalf("UnbindPaneEverywhere() error = %v", err)
	}

	chats, err := store.ListChats(ctx)
	if err != nil {
		t.Fatalf("ListChats() error = %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("chat count = %d, want 1", len(chats))
	}
	current, err := store.CurrentPane(ctx, 1)
	if err != nil {
		t.Fatalf("CurrentPane() error = %v", err)
	}
	if current != "" {
		t.Fatalf("current = %q, want empty", current)
	}
}

func TestStoreStatsAndMessages(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	if err := store.BindPane(ctx, 5, "default:%1"); err != nil {
		t.Fatalf("BindPane() error = %v", err)
	}
	if err := store.SetCurrentPane(ctx, 5, "default:%1"); err != nil {
		t.Fatalf("SetCurrentPane() error = %v", err)
	}
	if err := store.LogMessage(ctx, MessageRecord{
		ChatID:            5,
		PaneKey:           "default:%1",
		Direction:         "out",
		Kind:              "reply",
		TelegramMessageID: 88,
		BodyPreview:       "hello",
	}); err != nil {
		t.Fatalf("LogMessage() error = %v", err)
	}

	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats.Chats != 1 || stats.Bindings != 1 || stats.Messages != 1 {
		t.Fatalf("unexpected stats %#v", stats)
	}
}

func TestStoreMigratePhase2ToPhase3(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	if err := store.exec(ctx, `
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS message_links;
PRAGMA user_version = 1;
`); err != nil {
		t.Fatalf("prep old schema error = %v", err)
	}

	if err := store.applyMigrations(ctx); err != nil {
		t.Fatalf("applyMigrations() error = %v", err)
	}

	version, err := store.schemaVersion(ctx)
	if err != nil {
		t.Fatalf("schemaVersion() error = %v", err)
	}
	if version != schemaVersionPhase3 {
		t.Fatalf("schema version = %d, want %d", version, schemaVersionPhase3)
	}

	if _, err := store.EnsureSession(ctx, "telegram", 7, "default:%5", "codex"); err != nil {
		t.Fatalf("EnsureSession() error = %v", err)
	}
}

func TestStoreSessionLinksAndReplyTarget(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tagb.db")
	store, err := OpenStore(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	session, err := store.EnsureSession(ctx, "telegram", 7, "default:%5", "codex")
	if err != nil {
		t.Fatalf("EnsureSession() error = %v", err)
	}
	if session.SessionKey != "telegram:7:default:%5" {
		t.Fatalf("session key = %q", session.SessionKey)
	}

	sameSession, err := store.EnsureSession(ctx, "telegram", 7, "default:%5", "")
	if err != nil {
		t.Fatalf("EnsureSession() second error = %v", err)
	}
	if sameSession.SessionKey != session.SessionKey {
		t.Fatalf("session key changed: %q != %q", sameSession.SessionKey, session.SessionKey)
	}

	if err := store.TouchSessionInbound(ctx, session.SessionKey, 42); err != nil {
		t.Fatalf("TouchSessionInbound() error = %v", err)
	}
	if err := store.CreateMessageLink(ctx, MessageLinkRecord{
		Platform:         "telegram",
		ChatID:           7,
		PaneKey:          "default:%5",
		SessionKey:       session.SessionKey,
		Kind:             "command",
		InboundMessageID: 42,
	}); err != nil {
		t.Fatalf("CreateMessageLink(inbound) error = %v", err)
	}

	reloadedSession, err := store.SessionByKey(ctx, session.SessionKey)
	if err != nil {
		t.Fatalf("SessionByKey() error = %v", err)
	}
	if reloadedSession.LastInboundMessageID != 42 {
		t.Fatalf("last inbound = %d, want 42", reloadedSession.LastInboundMessageID)
	}

	if err := store.TouchSessionOutbound(ctx, session.SessionKey, 99); err != nil {
		t.Fatalf("TouchSessionOutbound() error = %v", err)
	}
	if err := store.CreateMessageLink(ctx, MessageLinkRecord{
		Platform:          "telegram",
		ChatID:            7,
		PaneKey:           "default:%5",
		SessionKey:        session.SessionKey,
		Kind:              "snapshot",
		OutboundMessageID: 99,
		ReplyToMessageID:  42,
	}); err != nil {
		t.Fatalf("CreateMessageLink(outbound) error = %v", err)
	}

	loaded, err := store.LatestSessionByChatPane(ctx, "telegram", 7, "default:%5")
	if err != nil {
		t.Fatalf("LatestSessionByChatPane() error = %v", err)
	}
	if loaded.LastInboundMessageID != 42 || loaded.LastOutboundMessageID != 99 {
		t.Fatalf("unexpected session %#v", loaded)
	}

	reopened, err := OpenStore(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenStore(reopen) error = %v", err)
	}
	reloadedAfterRestart, err := reopened.SessionByKey(ctx, session.SessionKey)
	if err != nil {
		t.Fatalf("SessionByKey(reopen) error = %v", err)
	}
	if reloadedAfterRestart.LastInboundMessageID != 42 {
		t.Fatalf("last inbound after reopen = %d, want 42", reloadedAfterRestart.LastInboundMessageID)
	}
}
