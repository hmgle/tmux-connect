package daemon

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hmgle/tmux-connect/internal/telegram"
	"github.com/hmgle/tmux-connect/internal/termrender"
	"github.com/hmgle/tmux-connect/internal/tmux"
)

func TestRouterFollow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	follow := NewFollowManager(service, replyBus, 20)
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, follow, 120, nil, "", "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/follow on")); err != nil {
		t.Fatalf("HandleMessage(follow on) error = %v", err)
	}

	service.sub.PushChunk(tmux.OutputChunk{Text: "delta output", ReceivedAt: time.Now()})
	waitForMessages(t, time.Second, func(messages []sentMessage) bool {
		for _, msg := range messages {
			if strings.Contains(msg.Text, "delta output") {
				return true
			}
		}
		return false
	}, messenger)
	follow.Disable(telegramChat(7).Key())

	messages := messenger.snapshot()
	texts := make([]string, 0, len(messages))
	for _, msg := range messages {
		texts = append(texts, msg.Text)
	}
	joined := strings.Join(texts, "\n")
	if !strings.Contains(joined, "follow enabled") {
		t.Fatalf("missing follow confirmation in %q", joined)
	}
	if !strings.Contains(joined, "initial output") {
		t.Fatalf("missing initial output in %q", joined)
	}
	if !strings.Contains(joined, "delta output") {
		t.Fatalf("missing streamed output in %q", joined)
	}
	for _, msg := range messages {
		if strings.Contains(msg.Text, "initial output") || strings.Contains(msg.Text, "delta output") {
			if msg.ParseMode != telegram.ParseModeHTML {
				t.Fatalf("follow output parse mode = %q, want %q for %#v", msg.ParseMode, telegram.ParseModeHTML, msg)
			}
		}
	}
	for _, msg := range messages[1:] {
		if msg.ReplyToMessageID != 2 {
			t.Fatalf("follow reply_to = %d, want 2 for %#v", msg.ReplyToMessageID, msg)
		}
	}
}

func TestRouterFollowFlushesBufferedOutputOnDisable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	follow := NewFollowManager(service, replyBus, 20)
	follow.minInterval = 5 * time.Second
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, follow, 120, nil, "", "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/follow on")); err != nil {
		t.Fatalf("HandleMessage(follow on) error = %v", err)
	}

	service.sub.PushChunk(tmux.OutputChunk{Text: "buffered before disable", ReceivedAt: time.Now()})
	if !follow.Disable(telegramChat(7).Key()) {
		t.Fatalf("Disable() = false, want true")
	}

	waitForMessages(t, time.Second, func(messages []sentMessage) bool {
		for _, msg := range messages {
			if strings.Contains(msg.Text, "buffered before disable") {
				return true
			}
		}
		return false
	}, messenger)
}

func TestRouterFollowSupportsCustomInterval(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	follow := NewFollowManager(service, replyBus, 20)
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, follow, 120, nil, "", "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/follow on 2s")); err != nil {
		t.Fatalf("HandleMessage(follow on 2s) error = %v", err)
	}

	if got := follow.Options(telegramChat(7).Key()).MinInterval; got != 2*time.Second {
		t.Fatalf("follow interval = %s, want %s", got, 2*time.Second)
	}

	messages := messenger.snapshot()
	found := false
	for _, msg := range messages {
		if strings.Contains(msg.Text, "min interval 2s") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("unexpected follow confirmation %#v", messages)
	}
}

func TestRouterFollowShowsContextForInlineUpdates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	service.initialOutput = "ready\ncalc> "
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	follow := NewFollowManager(service, replyBus, 20)
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, follow, 120, nil, "", "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/follow on 10ms")); err != nil {
		t.Fatalf("HandleMessage(follow on 10ms) error = %v", err)
	}

	service.sub.PushChunk(tmux.OutputChunk{Text: "1", ReceivedAt: time.Now()})
	waitForMessages(t, time.Second, func(messages []sentMessage) bool {
		for _, msg := range messages {
			if strings.Contains(msg.Text, "calc&gt; 1") {
				return true
			}
		}
		return false
	}, messenger)

	service.sub.PushChunk(tmux.OutputChunk{Text: "+", ReceivedAt: time.Now()})
	waitForMessages(t, time.Second, func(messages []sentMessage) bool {
		for _, msg := range messages {
			if strings.Contains(msg.Text, "calc&gt; 1+") {
				return true
			}
		}
		return false
	}, messenger)

	service.sub.PushChunk(tmux.OutputChunk{Text: "2", ReceivedAt: time.Now()})
	waitForMessages(t, time.Second, func(messages []sentMessage) bool {
		for _, msg := range messages {
			if strings.Contains(msg.Text, "calc&gt; 1+2") {
				return true
			}
		}
		return false
	}, messenger)

	messages := messenger.snapshot()
	latest := messages[len(messages)-1].Text
	if !strings.Contains(latest, "ready") {
		t.Fatalf("latest message = %q, want previous context", latest)
	}
	if !strings.Contains(latest, "calc&gt; 1+2") {
		t.Fatalf("latest message = %q, want complete updated line", latest)
	}
	if strings.HasSuffix(latest, "\n1") || strings.HasSuffix(latest, "\n+") || strings.HasSuffix(latest, "\n2") {
		t.Fatalf("latest message = %q, want contextual line instead of raw character delta", latest)
	}
}

func TestRouterFollowDrainsChunksAfterErrChannelCloses(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	follow := NewFollowManager(service, replyBus, 20)
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, follow, 120, nil, "", "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/follow on")); err != nil {
		t.Fatalf("HandleMessage(follow on) error = %v", err)
	}

	service.sub.CloseErrs()
	service.sub.PushChunk(tmux.OutputChunk{Text: "chunk after errs close", ReceivedAt: time.Now()})
	service.sub.CloseChunks()

	waitForMessages(t, time.Second, func(messages []sentMessage) bool {
		for _, msg := range messages {
			if strings.Contains(msg.Text, "chunk after errs close") {
				return true
			}
		}
		return false
	}, messenger)
}

func TestRouterClearStopsFollowAndKeepsSelectionHistory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	follow := NewFollowManager(service, replyBus, 20)
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, follow, 120, nil, "", "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/follow on")); err != nil {
		t.Fatalf("HandleMessage(follow on) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 3, "/clear")); err != nil {
		t.Fatalf("HandleMessage(clear) error = %v", err)
	}

	current, err := store.CurrentPane(ctx, telegramChat(7))
	if err != nil {
		t.Fatalf("CurrentPane() error = %v", err)
	}
	if current != "" {
		t.Fatalf("current = %q, want empty", current)
	}

	bindings, err := store.ListBindings(ctx, telegramChat(7))
	if err != nil {
		t.Fatalf("ListBindings() error = %v", err)
	}
	if len(bindings) != 1 || bindings[0] != "default:%5" {
		t.Fatalf("bindings = %#v, want [default:%%5]", bindings)
	}
	if follow.IsEnabled(telegramChat(7).Key()) {
		t.Fatalf("follow is still enabled")
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, "cleared current pane") {
		t.Fatalf("last message = %q, want clear confirmation", last.Text)
	}
}

func TestRouterUnmanageClearsBindingsAndFollow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	follow := NewFollowManager(service, replyBus, 20)
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, follow, 120, nil, "", "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/follow on")); err != nil {
		t.Fatalf("HandleMessage(follow on) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 3, "/unmanage %5")); err != nil {
		t.Fatalf("HandleMessage(unmanage) error = %v", err)
	}

	if service.detachCalls != 1 {
		t.Fatalf("detachCalls = %d, want 1", service.detachCalls)
	}

	current, err := store.CurrentPane(ctx, telegramChat(7))
	if err != nil {
		t.Fatalf("CurrentPane() error = %v", err)
	}
	if current != "" {
		t.Fatalf("current = %q, want empty", current)
	}

	bindings, err := store.ListBindings(ctx, telegramChat(7))
	if err != nil {
		t.Fatalf("ListBindings() error = %v", err)
	}
	if len(bindings) != 0 {
		t.Fatalf("bindings = %#v, want empty", bindings)
	}
	if follow.IsEnabled(telegramChat(7).Key()) {
		t.Fatalf("follow is still enabled")
	}

	record, err := service.Inspect(ctx, "%5")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if record.Metadata.Managed {
		t.Fatalf("managed = true, want false")
	}
}
