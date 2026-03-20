package daemon

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/hmgle/tmux-connect/internal/termrender"
)

func TestReplyBusAppliesInteractionReplyContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	messenger := &fakeMessenger{platform: "discord"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})

	err = replyBus.ReplyWithOptions(withInteractionReplyContext(ctx, "i-1"), discordChat("C123"), "", "help", "Commands", SendOptions{})
	if err != nil {
		t.Fatalf("ReplyWithOptions() error = %v", err)
	}

	messages := messenger.snapshot()
	if len(messages) != 1 {
		t.Fatalf("messages = %#v, want one message", messages)
	}
	if messages[0].InteractionID != "i-1" {
		t.Fatalf("InteractionID = %q, want i-1", messages[0].InteractionID)
	}
}
