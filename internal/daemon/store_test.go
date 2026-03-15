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
