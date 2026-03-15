package daemon

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portgle/tmux-connect/internal/tagb"
	"github.com/portgle/tmux-connect/internal/telegram"
	"github.com/portgle/tmux-connect/internal/tmux"
)

type fakeMessenger struct {
	mu       sync.Mutex
	messages []sentMessage
}

type sentMessage struct {
	Text             string
	ReplyToMessageID int64
}

func (m *fakeMessenger) SendMessage(_ context.Context, _ int64, text string, opts telegram.SendOptions) (telegram.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, sentMessage{
		Text:             text,
		ReplyToMessageID: opts.ReplyToMessageID,
	})
	return telegram.Message{MessageID: int64(len(m.messages))}, nil
}

func (m *fakeMessenger) snapshot() []sentMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]sentMessage, len(m.messages))
	copy(out, m.messages)
	return out
}

type fakePaneService struct {
	records map[string]tagb.PaneRecord
	sub     *tmux.Subscription
}

func newFakePaneService() *fakePaneService {
	record := tagb.PaneRecord{
		Info: tmux.PaneInfo{
			Target:      tmux.Target{Socket: "default", PaneID: "%5"},
			SessionName: "dev",
			WindowName:  "shell",
			CurrentCmd:  "codex",
		},
		Metadata: tmux.BridgeMetadata{Managed: true, Agent: tmux.AgentCodex, Mode: tmux.ModeRelay},
	}
	return &fakePaneService{
		records: map[string]tagb.PaneRecord{
			record.Info.Target.PaneKey(): record,
		},
		sub: tmux.NewSubscriptionForTest(),
	}
}

func (s *fakePaneService) List(context.Context) ([]tagb.PaneRecord, error) {
	out := make([]tagb.PaneRecord, 0, len(s.records))
	for _, record := range s.records {
		out = append(out, record)
	}
	return out, nil
}

func (s *fakePaneService) Attach(context.Context, string, string, string) (tagb.PaneRecord, error) {
	return s.records["default:%5"], nil
}

func (s *fakePaneService) Detach(context.Context, string) error { return nil }

func (s *fakePaneService) Inspect(_ context.Context, ref string) (tagb.PaneRecord, error) {
	if strings.HasPrefix(ref, "%") {
		ref = "default:" + ref
	}
	record := s.records[ref]
	return record, nil
}

func (s *fakePaneService) Snapshot(context.Context, string, int) (string, error) {
	return "hello from pane", nil
}

func (s *fakePaneService) Send(context.Context, string, string, bool) error { return nil }

func (s *fakePaneService) Enter(context.Context, string) error { return nil }

func (s *fakePaneService) CtrlC(context.Context, string) error { return nil }

func (s *fakePaneService) OpenStream(context.Context, string, int) (tagb.PaneStream, error) {
	return tagb.PaneStream{
		Pane:         s.records["default:%5"].Info,
		Initial:      "initial output",
		Subscription: s.sub,
	}, nil
}

func TestRouterBindAndSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store)
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil)

	if err := router.HandleMessage(ctx, IncomingMessage{ChatID: 7, MessageID: 1, Text: "/bind %5"}); err != nil {
		t.Fatalf("HandleMessage(bind) error = %v", err)
	}
	current, err := store.CurrentPane(ctx, 7)
	if err != nil {
		t.Fatalf("CurrentPane() error = %v", err)
	}
	if current != "default:%5" {
		t.Fatalf("current = %q, want %q", current, "default:%5")
	}

	if err := router.HandleMessage(ctx, IncomingMessage{ChatID: 7, MessageID: 2, Text: "/snapshot"}); err != nil {
		t.Fatalf("HandleMessage(snapshot) error = %v", err)
	}
	messages := messenger.snapshot()
	if len(messages) < 2 || !strings.Contains(messages[len(messages)-1].Text, "hello from pane") {
		t.Fatalf("unexpected messages %#v", messages)
	}
	if got := messages[len(messages)-1].ReplyToMessageID; got != 2 {
		t.Fatalf("snapshot reply_to = %d, want 2", got)
	}
}

func TestRouterFollow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store)
	follow := NewFollowManager(service, replyBus, 20)
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, follow, 120, nil)

	if err := router.HandleMessage(ctx, IncomingMessage{ChatID: 7, MessageID: 1, Text: "/bind %5"}); err != nil {
		t.Fatalf("HandleMessage(bind) error = %v", err)
	}
	if err := router.HandleMessage(ctx, IncomingMessage{ChatID: 7, MessageID: 2, Text: "/follow on"}); err != nil {
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
	follow.Disable(7)

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
	for _, msg := range messages[1:] {
		if msg.ReplyToMessageID != 2 {
			t.Fatalf("follow reply_to = %d, want 2 for %#v", msg.ReplyToMessageID, msg)
		}
	}
}

func TestRouterFollowFlushesBufferedOutputOnDisable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store)
	follow := NewFollowManager(service, replyBus, 20)
	follow.flushInterval = 5 * time.Second
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, follow, 120, nil)

	if err := router.HandleMessage(ctx, IncomingMessage{ChatID: 7, MessageID: 1, Text: "/bind %5"}); err != nil {
		t.Fatalf("HandleMessage(bind) error = %v", err)
	}
	if err := router.HandleMessage(ctx, IncomingMessage{ChatID: 7, MessageID: 2, Text: "/follow on"}); err != nil {
		t.Fatalf("HandleMessage(follow on) error = %v", err)
	}

	service.sub.PushChunk(tmux.OutputChunk{Text: "buffered before disable", ReceivedAt: time.Now()})
	if !follow.Disable(7) {
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

func waitForMessages(t *testing.T, timeout time.Duration, predicate func([]sentMessage) bool, messenger *fakeMessenger) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		messages := messenger.snapshot()
		if predicate(messages) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("condition not met before timeout, messages = %#v", messages)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
