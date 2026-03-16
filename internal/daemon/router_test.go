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
	Caption          string
	FileName         string
	Photo            []byte
	ParseMode        telegram.ParseMode
	ReplyToMessageID int64
}

func (m *fakeMessenger) SendMessage(_ context.Context, _ int64, text string, opts telegram.SendOptions) (telegram.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, sentMessage{
		Text:             text,
		ParseMode:        opts.ParseMode,
		ReplyToMessageID: opts.ReplyToMessageID,
	})
	return telegram.Message{MessageID: int64(len(m.messages))}, nil
}

func (m *fakeMessenger) SendPhoto(_ context.Context, _ int64, fileName string, photo []byte, caption string, opts telegram.SendOptions) (telegram.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, sentMessage{
		Caption:          caption,
		FileName:         fileName,
		Photo:            append([]byte(nil), photo...),
		ParseMode:        opts.ParseMode,
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
	records       map[string]tagb.PaneRecord
	sub           *tmux.Subscription
	snapshotText  string
	snapshotRich  string
	initialOutput string
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
		sub:           tmux.NewSubscriptionForTest(),
		snapshotText:  "hello from pane",
		snapshotRich:  "\x1b[32mhello from pane\x1b[0m",
		initialOutput: "initial output",
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
	return s.snapshotText, nil
}

func (s *fakePaneService) SnapshotRich(context.Context, string, int) (string, error) {
	return s.snapshotRich, nil
}

func (s *fakePaneService) Send(context.Context, string, string, bool) error { return nil }

func (s *fakePaneService) Enter(context.Context, string) error { return nil }

func (s *fakePaneService) CtrlC(context.Context, string) error { return nil }

func (s *fakePaneService) OpenStream(context.Context, string, int) (tagb.PaneStream, error) {
	return tagb.PaneStream{
		Pane:         s.records["default:%5"].Info,
		Initial:      s.initialOutput,
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
	if len(messages) < 2 || len(messages[len(messages)-1].Photo) == 0 {
		t.Fatalf("unexpected messages %#v", messages)
	}
	if got := messages[len(messages)-1].Caption; got != "default:%5 snapshot" {
		t.Fatalf("snapshot caption = %q, want %q", got, "default:%5 snapshot")
	}
	if got := messages[len(messages)-1].FileName; got != "pane-snapshot.png" {
		t.Fatalf("snapshot filename = %q, want %q", got, "pane-snapshot.png")
	}
	if got := messages[len(messages)-1].ReplyToMessageID; got != 2 {
		t.Fatalf("snapshot reply_to = %d, want 2", got)
	}
}

func TestRouterSnapshotTextMode(t *testing.T) {
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
	if err := router.HandleMessage(ctx, IncomingMessage{ChatID: 7, MessageID: 2, Text: "/snapshot text"}); err != nil {
		t.Fatalf("HandleMessage(snapshot text) error = %v", err)
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if len(last.Photo) != 0 {
		t.Fatalf("expected text snapshot, got photo message %#v", last)
	}
	if !strings.Contains(last.Text, "hello from pane") {
		t.Fatalf("snapshot text = %q, want pane text", last.Text)
	}
	if got := last.ParseMode; got != telegram.ParseModeHTML {
		t.Fatalf("snapshot text parse mode = %q, want %q", got, telegram.ParseModeHTML)
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
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store)
	follow := NewFollowManager(service, replyBus, 20)
	follow.minInterval = 5 * time.Second
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

func TestRouterFollowSupportsCustomInterval(t *testing.T) {
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
	if err := router.HandleMessage(ctx, IncomingMessage{ChatID: 7, MessageID: 2, Text: "/follow on 2s"}); err != nil {
		t.Fatalf("HandleMessage(follow on 2s) error = %v", err)
	}

	if got := follow.Options(7).MinInterval; got != 2*time.Second {
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
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	service.initialOutput = "ready\ncalc> "
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store)
	follow := NewFollowManager(service, replyBus, 20)
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, follow, 120, nil)

	if err := router.HandleMessage(ctx, IncomingMessage{ChatID: 7, MessageID: 1, Text: "/bind %5"}); err != nil {
		t.Fatalf("HandleMessage(bind) error = %v", err)
	}
	if err := router.HandleMessage(ctx, IncomingMessage{ChatID: 7, MessageID: 2, Text: "/follow on 10ms"}); err != nil {
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

func TestParseSnapshotArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		value     string
		wantLines int
		wantMode  snapshotMode
		wantErr   bool
	}{
		{name: "default", value: "", wantLines: 120, wantMode: snapshotModeImage},
		{name: "lines only", value: "200", wantLines: 200, wantMode: snapshotModeImage},
		{name: "text only", value: "text", wantLines: 120, wantMode: snapshotModeText},
		{name: "image only", value: "image", wantLines: 120, wantMode: snapshotModeImage},
		{name: "lines then text", value: "200 text", wantLines: 200, wantMode: snapshotModeText},
		{name: "text then lines", value: "text 200", wantLines: 200, wantMode: snapshotModeText},
		{name: "bad mode", value: "plain", wantErr: true},
		{name: "bad lines", value: "0", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines, mode, err := parseSnapshotArgs(tt.value, 120)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseSnapshotArgs(%q) error = nil, want error", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSnapshotArgs(%q) error = %v", tt.value, err)
			}
			if lines != tt.wantLines {
				t.Fatalf("lines = %d, want %d", lines, tt.wantLines)
			}
			if mode != tt.wantMode {
				t.Fatalf("mode = %q, want %q", mode, tt.wantMode)
			}
		})
	}
}
