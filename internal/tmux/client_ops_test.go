package tmux

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"
)

type runnerCall struct {
	stdin []byte
	args  []string
}

type fakeRunner struct {
	runFn      func(context.Context, []byte, ...string) (string, error)
	startPTYFn func(context.Context, ...string) (PTYSession, error)
	calls      []runnerCall
}

func (r *fakeRunner) Run(ctx context.Context, stdin []byte, args ...string) (string, error) {
	call := runnerCall{
		stdin: append([]byte(nil), stdin...),
		args:  append([]string(nil), args...),
	}
	r.calls = append(r.calls, call)
	if r.runFn != nil {
		return r.runFn(ctx, stdin, args...)
	}
	return "", nil
}

func (r *fakeRunner) StartPTY(ctx context.Context, args ...string) (PTYSession, error) {
	if r.startPTYFn != nil {
		return r.startPTYFn(ctx, args...)
	}
	return nil, errors.New("not implemented")
}

func TestListPaneStatesUsesSingleCommand(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		runFn: func(_ context.Context, _ []byte, args ...string) (string, error) {
			if len(args) < 4 || args[0] != "list-panes" {
				t.Fatalf("unexpected command: %v", args)
			}
			return "%5\x1fdev\x1f@1\x1fshell\x1fapi\x1fzsh\x1f0\x1f120\x1f40\x1f1\x1frelay\x1fcodex\x1fbackend\x1fmanual-attach\x1f1700000000\n", nil
		},
	}
	client := NewClient(runner, "")

	states, err := client.ListPaneStates(context.Background())
	if err != nil {
		t.Fatalf("ListPaneStates() error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 tmux call, got %d", len(runner.calls))
	}
	if len(states) != 1 {
		t.Fatalf("expected 1 pane state, got %d", len(states))
	}
	got := states[0]
	if got.Info.Target.PaneKey() != "default:%5" {
		t.Fatalf("unexpected pane key %q", got.Info.Target.PaneKey())
	}
	if !got.Metadata.Managed || got.Metadata.Agent != AgentCodex || got.Metadata.Label != "backend" {
		t.Fatalf("unexpected metadata %#v", got.Metadata)
	}
}

func TestParsePaneInfoLineAcceptsEscapedSeparator(t *testing.T) {
	t.Parallel()

	line := `%1\0372\037@1\037zsh\037gle@host:~/proj\037zsh\0370\03780\03722`

	pane, err := parsePaneInfoLine("default", line)
	if err != nil {
		t.Fatalf("parsePaneInfoLine() error = %v", err)
	}
	if pane.Target.PaneID != "%1" || pane.SessionName != "2" {
		t.Fatalf("unexpected pane %#v", pane)
	}
	if pane.Width != 80 || pane.Height != 22 {
		t.Fatalf("unexpected size %#v", pane)
	}
}

func TestParsePaneStateLineAcceptsEscapedSeparator(t *testing.T) {
	t.Parallel()

	line := `%5\037dev\037@1\037shell\037api\037zsh\0370\037120\03740\0371\037relay\037codex\037backend\037manual-attach\0371700000000`

	pane, meta, err := parsePaneStateLine("default", line)
	if err != nil {
		t.Fatalf("parsePaneStateLine() error = %v", err)
	}
	if pane.Target.PaneID != "%5" || pane.WindowName != "shell" {
		t.Fatalf("unexpected pane %#v", pane)
	}
	if !meta.Managed || meta.Agent != AgentCodex || meta.Label != "backend" {
		t.Fatalf("unexpected metadata %#v", meta)
	}
}

func TestInjectInputUsesNamedBuffer(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	client := NewClient(runner, "")

	err := client.InjectInput(context.Background(), Target{PaneID: "%5"}, []byte("hello"))
	if err != nil {
		t.Fatalf("InjectInput() error = %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 tmux calls, got %d", len(runner.calls))
	}
	if got := runner.calls[0].args; len(got) < 4 || got[0] != "load-buffer" || got[1] != "-b" || got[2] != "tagb-5" || got[3] != "-" {
		t.Fatalf("unexpected load-buffer args: %v", got)
	}
	if got := runner.calls[1].args; len(got) < 7 || got[0] != "paste-buffer" || got[1] != "-b" || got[2] != "tagb-5" {
		t.Fatalf("unexpected paste-buffer args: %v", got)
	}
}

func TestTouchMetadataUpdatesLastActivityOnlyForManagedPanes(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		runFn: func(_ context.Context, _ []byte, args ...string) (string, error) {
			switch args[0] {
			case "show-options":
				return "1\n", nil
			case "set-option":
				return "", nil
			default:
				t.Fatalf("unexpected command: %v", args)
				return "", nil
			}
		},
	}
	client := NewClient(runner, "")

	err := client.TouchMetadata(context.Background(), Target{PaneID: "%7"})
	if err != nil {
		t.Fatalf("TouchMetadata() error = %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 tmux calls, got %d", len(runner.calls))
	}
	showCall := runner.calls[0].args
	if got := showCall[:4]; got[0] != "show-options" || got[1] != "-p" || got[2] != "-v" || got[3] != "-t" {
		t.Fatalf("unexpected show-options args: %v", showCall)
	}
	setCall := runner.calls[1].args
	if len(setCall) != 6 || setCall[0] != "set-option" || setCall[4] != OptionLastActivity {
		t.Fatalf("unexpected set-option args: %v", setCall)
	}
	if _, err := strconv.ParseInt(setCall[5], 10, 64); err != nil {
		t.Fatalf("last activity is not unix time: %q", setCall[5])
	}
}

func TestDeleteUserOptionIgnoresUnsetErrors(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		runFn: func(_ context.Context, _ []byte, _ ...string) (string, error) {
			return "", errors.New("exit status 1: unknown option")
		},
	}
	client := NewClient(runner, "")

	if err := client.DeleteUserOption(context.Background(), Target{PaneID: "%1"}, OptionManaged); err != nil {
		t.Fatalf("DeleteUserOption() error = %v", err)
	}
}

func TestStartPollingSubscriptionHonorsLinesAndClose(t *testing.T) {
	t.Parallel()

	captured := make(chan []string, 1)
	runner := &fakeRunner{
		runFn: func(_ context.Context, _ []byte, args ...string) (string, error) {
			if args[0] != "capture-pane" {
				t.Fatalf("unexpected command: %v", args)
			}
			captured <- append([]string(nil), args...)
			return "one\ntwo", nil
		},
	}
	client := NewClient(runner, "")

	sub := client.startPollingSubscription(context.Background(), PaneInfo{Target: Target{PaneID: "%3"}}, 42)
	args := <-captured
	if got, want := args[len(args)-1], "-41"; got != want {
		t.Fatalf("capture-pane start = %q, want %q", got, want)
	}
	if err := sub.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case _, ok := <-sub.Chunks():
		if ok {
			t.Fatal("expected chunks channel to close after Close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("polling subscription did not stop after Close")
	}
}

func TestOpenPaneStreamSeedsPollingWithInitialSnapshot(t *testing.T) {
	t.Parallel()

	captures := []string{"one", "one\ntwo"}
	var captureIndex int
	runner := &fakeRunner{
		runFn: func(_ context.Context, _ []byte, args ...string) (string, error) {
			if args[0] != "capture-pane" {
				t.Fatalf("unexpected command: %v", args)
			}
			if captureIndex >= len(captures) {
				return captures[len(captures)-1], nil
			}
			body := captures[captureIndex]
			captureIndex++
			return body, nil
		},
		startPTYFn: func(context.Context, ...string) (PTYSession, error) {
			return nil, errors.New("control unavailable")
		},
	}
	client := NewClient(runner, "")
	pane := PaneInfo{
		Target:      Target{PaneID: "%9"},
		SessionName: "dev",
	}

	initial, sub, err := client.OpenPaneStream(context.Background(), pane, 20)
	if err != nil {
		t.Fatalf("OpenPaneStream() error = %v", err)
	}
	defer sub.Close()

	if initial != "one" {
		t.Fatalf("initial = %q, want %q", initial, "one")
	}
	select {
	case chunk := <-sub.Chunks():
		if chunk.Text != "two" {
			t.Fatalf("chunk.Text = %q, want %q", chunk.Text, "two")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected polling diff chunk")
	}
}

func TestCapturePaneRichUsesEscapeFlag(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	client := NewClient(runner, "")

	if _, err := client.CapturePaneRich(context.Background(), Target{PaneID: "%5"}, 20); err != nil {
		t.Fatalf("CapturePaneRich() error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 tmux call, got %d", len(runner.calls))
	}
	got := runner.calls[0].args
	want := []string{"capture-pane", "-p", "-J", "-e", "-t", "%5", "-S", "-19"}
	if len(got) != len(want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q in %v", i, got[i], want[i], got)
		}
	}
}
