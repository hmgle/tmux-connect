package daemon

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hmgle/tmux-connect/internal/termrender"
	"github.com/hmgle/tmux-connect/internal/tmux"
	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

func TestFormatFollowMessageKeepsTailWhenTruncated(t *testing.T) {
	t.Parallel()

	text := strings.Join([]string{
		"header line",
		"noise line 1",
		"noise line 2",
		"To continue this session, run /resume",
	}, "\n")

	got := formatFollowMessage("default:%5", text, 60)

	if !strings.HasPrefix(got, "[default:%5]\n...[truncated]\n") {
		t.Fatalf("formatFollowMessage() prefix = %q", got)
	}
	if !strings.Contains(got, "To continue this session, run /resume") {
		t.Fatalf("formatFollowMessage() = %q, want preserved tail", got)
	}
	if strings.Contains(got, "header line") {
		t.Fatalf("formatFollowMessage() = %q, want leading content trimmed", got)
	}
}

func TestFormatFollowMessageEmptyAfterTrim(t *testing.T) {
	t.Parallel()

	got := formatFollowMessage("default:%5", "  \n\t ", 20)
	want := "[default:%5] (empty output)"
	if got != want {
		t.Fatalf("formatFollowMessage() = %q, want %q", got, want)
	}
}

func TestBuildFollowUpdateSkipsExactDuplicate(t *testing.T) {
	t.Parallel()

	got, changed := buildFollowUpdate("same output", "same output")
	if changed {
		t.Fatalf("buildFollowUpdate() changed = true, got %q", got)
	}
	if got != "" {
		t.Fatalf("buildFollowUpdate() = %q, want empty", got)
	}
}

func TestBuildFollowUpdateShowsInlineContext(t *testing.T) {
	t.Parallel()

	previous := strings.Join([]string{
		"ready",
		"calc> ",
	}, "\n")
	current := previous + "1+2"

	got, changed := buildFollowUpdate(previous, current)
	if !changed {
		t.Fatal("buildFollowUpdate() changed = false, want true")
	}
	if !strings.Contains(got, "ready") {
		t.Fatalf("buildFollowUpdate() = %q, want prior context", got)
	}
	if !strings.Contains(got, "calc> 1+2") {
		t.Fatalf("buildFollowUpdate() = %q, want full updated line", got)
	}
	if strings.Contains(got, "\n1+2") {
		t.Fatalf("buildFollowUpdate() = %q, want contextual line instead of bare suffix", got)
	}
}

func TestBuildFollowUpdateReturnsDeltaForCompletedLines(t *testing.T) {
	t.Parallel()

	previous := "line one\n"
	current := "line one\nline two\nline three\n"

	got, changed := buildFollowUpdate(previous, current)
	if !changed {
		t.Fatal("buildFollowUpdate() changed = false, want true")
	}
	want := "line two\nline three"
	if got != want {
		t.Fatalf("buildFollowUpdate() = %q, want %q", got, want)
	}
}

func TestBuildRecentFollowContextKeepsTailOfLongLine(t *testing.T) {
	t.Parallel()

	line := strings.Repeat("prefix-", 80) + "calc> 1+2"
	got := buildRecentFollowContext(line, 4, 40)

	if !strings.Contains(got, "calc> 1+2") {
		t.Fatalf("buildRecentFollowContext() = %q, want tail of long line", got)
	}
	if strings.Contains(got, "prefix-prefix-prefix-prefix-prefix") {
		t.Fatalf("buildRecentFollowContext() = %q, want long prefix trimmed", got)
	}
}

func TestBuildRecentFollowContextUsesLastCarriageReturnSegment(t *testing.T) {
	t.Parallel()

	text := "old prompt\rready\n>>> 1+2\r>>> 1+2\n3\n>>> "
	got := buildRecentFollowContext(text, 4, 80)

	if strings.Contains(got, "old prompt") {
		t.Fatalf("buildRecentFollowContext() = %q, want overwritten carriage-return content removed", got)
	}
	if !strings.Contains(got, ">>> 1+2") {
		t.Fatalf("buildRecentFollowContext() = %q, want latest visible prompt", got)
	}
}

func TestBuildFollowUpdatePrefersContextOverHugeRedrawDelta(t *testing.T) {
	t.Parallel()

	previous := strings.Join([]string{
		"old summary line",
		">>> 1+2",
		"3",
		">>> 4+5",
		"9",
		">>> 6+",
	}, "\n")
	redraw := strings.Join([]string{
		"----------------------------------------",
		"old summary line",
		">>> 1+2",
		"3",
		">>> 4+5",
		"9",
		">>> 6+7",
		"13",
		">>> ",
	}, "\n")
	current := previous + redraw

	got, changed := buildFollowUpdate(previous, current)
	if !changed {
		t.Fatal("buildFollowUpdate() changed = false, want true")
	}
	if strings.Contains(got, "old summary line") {
		t.Fatalf("buildFollowUpdate() = %q, want large redraw prefix omitted", got)
	}
	if !strings.Contains(got, ">>> 6+7") {
		t.Fatalf("buildFollowUpdate() = %q, want latest prompt context", got)
	}
	if !strings.Contains(got, "13") {
		t.Fatalf("buildFollowUpdate() = %q, want latest result line", got)
	}
}

func TestFollowManagerDisableFlushesBufferedOutputBeforeReturning(t *testing.T) {
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

	if err := follow.Enable(ctx, telegramChat(7), "default:%5"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	service.sub.PushChunk(tmux.OutputChunk{Text: "buffered before disable", ReceivedAt: time.Now()})
	if !follow.Disable(telegramChat(7).Key()) {
		t.Fatal("Disable() = false, want true")
	}

	joined := joinSentTexts(messenger.snapshot())
	if !strings.Contains(joined, "buffered before disable") {
		t.Fatalf("messages = %q, want flushed buffered output", joined)
	}
}

func TestFollowManagerReplacingSessionKeepsNewestSessionAndDropsStaleBuffer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	base := newFakePaneService()
	info := base.records["default:%5"].Info
	sub1 := tmux.NewSubscriptionForTest()
	sub2 := tmux.NewSubscriptionForTest()
	service := &queuedFollowPaneService{
		fakePaneService: base,
		streams: []tmuxconn.PaneStream{
			{Pane: info, Subscription: sub1},
			{Pane: info, Subscription: sub2},
		},
	}
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	follow := NewFollowManager(service, replyBus, 20)
	follow.minInterval = 5 * time.Second

	chat := telegramChat(7)
	if err := follow.Enable(ctx, chat, "default:%5"); err != nil {
		t.Fatalf("first Enable() error = %v", err)
	}
	sub1.PushChunk(tmux.OutputChunk{Text: "stale buffered output", ReceivedAt: time.Now()})

	if err := follow.Enable(ctx, chat, "default:%5"); err != nil {
		t.Fatalf("second Enable() error = %v", err)
	}
	if !follow.IsEnabled(chat.Key()) {
		t.Fatal("follow session disappeared after replacement")
	}

	sub2.PushChunk(tmux.OutputChunk{Text: "fresh buffered output", ReceivedAt: time.Now()})
	if !follow.Disable(chat.Key()) {
		t.Fatal("Disable() = false, want true")
	}

	joined := joinSentTexts(messenger.snapshot())
	if strings.Contains(joined, "stale buffered output") {
		t.Fatalf("messages = %q, want stale buffered output dropped on replacement", joined)
	}
	if !strings.Contains(joined, "fresh buffered output") {
		t.Fatalf("messages = %q, want fresh buffered output from replacement session", joined)
	}
}

type queuedFollowPaneService struct {
	*fakePaneService
	mu      sync.Mutex
	streams []tmuxconn.PaneStream
}

func (s *queuedFollowPaneService) OpenStream(ctx context.Context, paneKey string, lines int) (tmuxconn.PaneStream, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.streams) == 0 {
		return s.fakePaneService.OpenStream(ctx, paneKey, lines)
	}
	stream := s.streams[0]
	s.streams = s.streams[1:]
	return stream, nil
}

func joinSentTexts(messages []sentMessage) string {
	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		if msg.Text == "" {
			continue
		}
		parts = append(parts, msg.Text)
	}
	return strings.Join(parts, "\n---\n")
}
