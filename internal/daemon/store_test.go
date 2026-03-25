package daemon

import (
	"context"
	"path/filepath"
	"testing"
)

func TestStoreBindAndCurrentPane(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	chat := telegramChat(101)

	if err := store.BindPane(ctx, chat, "default:%5"); err != nil {
		t.Fatalf("BindPane() error = %v", err)
	}
	if err := store.BindPane(ctx, chat, "default:%8"); err != nil {
		t.Fatalf("BindPane() second error = %v", err)
	}
	if err := store.SetCurrentPane(ctx, chat, "default:%8"); err != nil {
		t.Fatalf("SetCurrentPane() error = %v", err)
	}

	bindings, err := store.ListBindings(ctx, chat)
	if err != nil {
		t.Fatalf("ListBindings() error = %v", err)
	}
	if len(bindings) != 2 {
		t.Fatalf("bindings len = %d, want 2", len(bindings))
	}
	current, err := store.CurrentPane(ctx, chat)
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
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	chat1 := telegramChat(1)
	chat2 := telegramChat(2)

	if err := store.BindPane(ctx, chat1, "default:%5"); err != nil {
		t.Fatalf("BindPane() error = %v", err)
	}
	if err := store.SetCurrentPane(ctx, chat1, "default:%5"); err != nil {
		t.Fatalf("SetCurrentPane() error = %v", err)
	}
	if err := store.BindPane(ctx, chat2, "default:%5"); err != nil {
		t.Fatalf("BindPane() second error = %v", err)
	}

	if err := store.UnbindPaneEverywhere(ctx, "default:%5"); err != nil {
		t.Fatalf("UnbindPaneEverywhere() error = %v", err)
	}

	chats, err := store.ListChats(ctx, "telegram")
	if err != nil {
		t.Fatalf("ListChats() error = %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("chat count = %d, want 1", len(chats))
	}
	current, err := store.CurrentPane(ctx, chat1)
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
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	chat := telegramChat(5)

	if err := store.BindPane(ctx, chat, "default:%1"); err != nil {
		t.Fatalf("BindPane() error = %v", err)
	}
	if err := store.SetCurrentPane(ctx, chat, "default:%1"); err != nil {
		t.Fatalf("SetCurrentPane() error = %v", err)
	}
	if err := store.LogMessage(ctx, MessageRecord{
		Chat:              chat,
		PaneKey:           "default:%1",
		Direction:         "out",
		Kind:              "reply",
		PlatformMessageID: "88",
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

func TestStoreHasThread(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	chat := ChatRef{Platform: "slack", ChatID: "C123"}

	if err := store.LogMessage(ctx, MessageRecord{
		Chat:              chat,
		Direction:         "out",
		Kind:              "reply",
		PlatformMessageID: "m1",
		ThreadID:          "thread-1",
		BodyPreview:       "hello",
	}); err != nil {
		t.Fatalf("LogMessage() error = %v", err)
	}

	ok, err := store.HasThread(ctx, chat, "thread-1")
	if err != nil {
		t.Fatalf("HasThread() error = %v", err)
	}
	if !ok {
		t.Fatal("HasThread() = false, want true")
	}

	ok, err = store.HasThread(ctx, chat, "missing-thread")
	if err != nil {
		t.Fatalf("HasThread(missing) error = %v", err)
	}
	if ok {
		t.Fatal("HasThread(missing) = true, want false")
	}
}

func TestStorePlatformRuntimeState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	if err := store.SetPlatformRuntimeState(ctx, "weixin", "context_token", "user@im.wechat", "ctx-1"); err != nil {
		t.Fatalf("SetPlatformRuntimeState() error = %v", err)
	}
	if err := store.SetPlatformRuntimeState(ctx, "weixin", "cursor", "", "cursor-2"); err != nil {
		t.Fatalf("SetPlatformRuntimeState(cursor) error = %v", err)
	}

	token, err := store.GetPlatformRuntimeState(ctx, "weixin", "context_token", "user@im.wechat")
	if err != nil {
		t.Fatalf("GetPlatformRuntimeState(token) error = %v", err)
	}
	if token != "ctx-1" {
		t.Fatalf("context token = %q, want ctx-1", token)
	}

	cursor, err := store.GetPlatformRuntimeState(ctx, "weixin", "cursor", "")
	if err != nil {
		t.Fatalf("GetPlatformRuntimeState(cursor) error = %v", err)
	}
	if cursor != "cursor-2" {
		t.Fatalf("cursor = %q, want cursor-2", cursor)
	}
}

func TestStoreMigratePhase2ToPhase3(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
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
	if version != schemaVersionPhase6 {
		t.Fatalf("schema version = %d, want %d", version, schemaVersionPhase6)
	}

	if _, err := store.EnsureSession(ctx, telegramChat(7), "default:%5", "codex"); err != nil {
		t.Fatalf("EnsureSession() error = %v", err)
	}
}

func TestStoreSessionLinksAndReplyTarget(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tmuxconn.db")
	store, err := OpenStore(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	chat := telegramChat(7)
	session, err := store.EnsureSession(ctx, chat, "default:%5", "codex")
	if err != nil {
		t.Fatalf("EnsureSession() error = %v", err)
	}
	if session.SessionKey != "telegram:7:default:%5" {
		t.Fatalf("session key = %q", session.SessionKey)
	}

	sameSession, err := store.EnsureSession(ctx, chat, "default:%5", "")
	if err != nil {
		t.Fatalf("EnsureSession() second error = %v", err)
	}
	if sameSession.SessionKey != session.SessionKey {
		t.Fatalf("session key changed: %q != %q", sameSession.SessionKey, session.SessionKey)
	}

	if err := store.TouchSessionInbound(ctx, session.SessionKey, "42"); err != nil {
		t.Fatalf("TouchSessionInbound() error = %v", err)
	}
	if err := store.CreateMessageLink(ctx, MessageLinkRecord{
		Platform:         "telegram",
		ChatID:           chat.ChatID,
		PaneKey:          "default:%5",
		SessionKey:       session.SessionKey,
		Kind:             "command",
		InboundMessageID: "42",
	}); err != nil {
		t.Fatalf("CreateMessageLink(inbound) error = %v", err)
	}

	reloadedSession, err := store.SessionByKey(ctx, session.SessionKey)
	if err != nil {
		t.Fatalf("SessionByKey() error = %v", err)
	}
	if reloadedSession.LastInboundMessageID != "42" {
		t.Fatalf("last inbound = %q, want 42", reloadedSession.LastInboundMessageID)
	}

	if err := store.TouchSessionOutbound(ctx, session.SessionKey, "99"); err != nil {
		t.Fatalf("TouchSessionOutbound() error = %v", err)
	}
	if err := store.CreateMessageLink(ctx, MessageLinkRecord{
		Platform:          "telegram",
		ChatID:            chat.ChatID,
		PaneKey:           "default:%5",
		SessionKey:        session.SessionKey,
		Kind:              "snapshot",
		OutboundMessageID: "99",
		ReplyToMessageID:  "42",
	}); err != nil {
		t.Fatalf("CreateMessageLink(outbound) error = %v", err)
	}

	loaded, err := store.LatestSessionByChatPane(ctx, chat, "default:%5")
	if err != nil {
		t.Fatalf("LatestSessionByChatPane() error = %v", err)
	}
	if loaded.LastInboundMessageID != "42" || loaded.LastOutboundMessageID != "99" {
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
	if reloadedAfterRestart.LastInboundMessageID != "42" {
		t.Fatalf("last inbound after reopen = %q, want 42", reloadedAfterRestart.LastInboundMessageID)
	}
}

func TestStoreSupportsSlackStringChatIDs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	chat := ChatRef{Platform: "slack", ChatID: "C12345"}
	if err := store.BindPane(ctx, chat, "default:%5"); err != nil {
		t.Fatalf("BindPane() error = %v", err)
	}
	if err := store.SetCurrentPane(ctx, chat, "default:%5"); err != nil {
		t.Fatalf("SetCurrentPane() error = %v", err)
	}
	session, err := store.EnsureSession(ctx, chat, "default:%5", "codex")
	if err != nil {
		t.Fatalf("EnsureSession() error = %v", err)
	}
	if session.SessionKey != "slack:C12345:default:%5" {
		t.Fatalf("session key = %q, want slack:C12345:default:%%5", session.SessionKey)
	}
	current, err := store.CurrentPane(ctx, chat)
	if err != nil {
		t.Fatalf("CurrentPane() error = %v", err)
	}
	if current != "default:%5" {
		t.Fatalf("current = %q, want default:%%5", current)
	}
}
