package daemon

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portgle/tmux-connect/internal/tagb"
	"github.com/portgle/tmux-connect/internal/telegram"
	"github.com/portgle/tmux-connect/internal/tmux"
)

type fakeMessenger struct {
	messages []string
}

func (m *fakeMessenger) SendMessage(_ context.Context, _ int64, text string) (telegram.Message, error) {
	m.messages = append(m.messages, text)
	return telegram.Message{MessageID: int64(len(m.messages))}, nil
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
	if len(messenger.messages) < 2 || !strings.Contains(messenger.messages[len(messenger.messages)-1], "hello from pane") {
		t.Fatalf("unexpected messages %#v", messenger.messages)
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
	time.Sleep(900 * time.Millisecond)
	follow.Disable(7)

	joined := strings.Join(messenger.messages, "\n")
	if !strings.Contains(joined, "follow enabled") {
		t.Fatalf("missing follow confirmation in %q", joined)
	}
	if !strings.Contains(joined, "initial output") {
		t.Fatalf("missing initial output in %q", joined)
	}
	if !strings.Contains(joined, "delta output") {
		t.Fatalf("missing streamed output in %q", joined)
	}
}
