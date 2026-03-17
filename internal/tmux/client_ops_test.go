package tmux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type runnerCall struct {
	stdin []byte
	args  []string
}

type fakeRunner struct {
	runFn      func(context.Context, []byte, ...string) (RunResult, error)
	startPTYFn func(context.Context, ...string) (PTYSession, error)
	calls      []runnerCall
}

type fakePTYSession struct {
	name      string
	closeFn   func() error
	waitFn    func() error
	closeHits atomic.Int32
	waitHits  atomic.Int32
}

func stdoutResult(stdout string) RunResult {
	return RunResult{Stdout: stdout}
}

func (r *fakeRunner) Run(ctx context.Context, stdin []byte, args ...string) (RunResult, error) {
	call := runnerCall{
		stdin: append([]byte(nil), stdin...),
		args:  append([]string(nil), args...),
	}
	r.calls = append(r.calls, call)
	if r.runFn != nil {
		return r.runFn(ctx, stdin, args...)
	}
	return RunResult{}, nil
}

func (r *fakeRunner) StartPTY(ctx context.Context, args ...string) (PTYSession, error) {
	if r.startPTYFn != nil {
		return r.startPTYFn(ctx, args...)
	}
	return nil, errors.New("not implemented")
}

func (s *fakePTYSession) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (s *fakePTYSession) Write(p []byte) (int, error) { return len(p), nil }
func (s *fakePTYSession) Close() error {
	s.closeHits.Add(1)
	if s.closeFn != nil {
		return s.closeFn()
	}
	return nil
}
func (s *fakePTYSession) Name() string { return s.name }
func (s *fakePTYSession) Wait() error {
	s.waitHits.Add(1)
	if s.waitFn != nil {
		return s.waitFn()
	}
	return nil
}

func TestListPaneStatesUsesSingleCommand(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		runFn: func(_ context.Context, _ []byte, args ...string) (RunResult, error) {
			if len(args) < 4 || args[0] != "list-panes" {
				t.Fatalf("unexpected command: %v", args)
			}
			return stdoutResult("%5\x1fdev\x1f@1\x1fshell\x1fapi\x1fzsh\x1f0\x1f120\x1f40\x1f1\x1frelay\x1fcodex\x1fbackend\x1fmanual-attach\x1f1700000000\n"), nil
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

func TestGetPaneUsesTargetedListCommand(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		runFn: func(_ context.Context, _ []byte, args ...string) (RunResult, error) {
			if len(args) < 4 || args[0] != "list-panes" || args[1] != "-t" || args[2] != "%5" {
				t.Fatalf("unexpected command: %v", args)
			}
			return stdoutResult("%5\x1fdev\x1f@1\x1fshell\x1fapi\x1fzsh\x1f0\x1f120\x1f40\n"), nil
		},
	}
	client := NewClient(runner, "")

	pane, err := client.GetPane(context.Background(), Target{PaneID: "%5"})
	if err != nil {
		t.Fatalf("GetPane() error = %v", err)
	}
	if pane.Target.PaneID != "%5" || pane.SessionName != "dev" {
		t.Fatalf("unexpected pane %#v", pane)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 tmux call, got %d", len(runner.calls))
	}
}

func TestGetPaneFiltersTargetedListToRequestedPane(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		runFn: func(_ context.Context, _ []byte, args ...string) (RunResult, error) {
			if len(args) < 4 || args[0] != "list-panes" || args[1] != "-t" || args[2] != "%507" {
				t.Fatalf("unexpected command: %v", args)
			}
			return stdoutResult(strings.Join([]string{
				"%489\x1fdev\x1f@1\x1fshell\x1fleft\x1fzsh\x1f0\x1f120\x1f40",
				"%507\x1fdev\x1f@1\x1fshell\x1fright\x1fcodex\x1f0\x1f120\x1f40",
			}, "\n") + "\n"), nil
		},
	}
	client := NewClient(runner, "")

	pane, err := client.GetPane(context.Background(), Target{PaneID: "%507"})
	if err != nil {
		t.Fatalf("GetPane() error = %v", err)
	}
	if pane.Target.PaneID != "%507" || pane.PaneTitle != "right" {
		t.Fatalf("unexpected pane %#v", pane)
	}
}

func TestGetPaneStateUsesTargetedListCommand(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		runFn: func(_ context.Context, _ []byte, args ...string) (RunResult, error) {
			if len(args) < 4 || args[0] != "list-panes" || args[1] != "-t" || args[2] != "%5" {
				t.Fatalf("unexpected command: %v", args)
			}
			return stdoutResult("%5\x1fdev\x1f@1\x1fshell\x1fapi\x1fzsh\x1f0\x1f120\x1f40\x1f1\x1frelay\x1fcodex\x1fbackend\x1fmanual-attach\x1f1700000000\n"), nil
		},
	}
	client := NewClient(runner, "")

	state, err := client.GetPaneState(context.Background(), Target{PaneID: "%5"})
	if err != nil {
		t.Fatalf("GetPaneState() error = %v", err)
	}
	if state.Info.Target.PaneID != "%5" || !state.Metadata.Managed || state.Metadata.Agent != AgentCodex {
		t.Fatalf("unexpected state %#v", state)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 tmux call, got %d", len(runner.calls))
	}
}

func TestGetPaneStateFiltersTargetedListToRequestedPane(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		runFn: func(_ context.Context, _ []byte, args ...string) (RunResult, error) {
			if len(args) < 4 || args[0] != "list-panes" || args[1] != "-t" || args[2] != "%507" {
				t.Fatalf("unexpected command: %v", args)
			}
			return stdoutResult(strings.Join([]string{
				"%489\x1fdev\x1f@1\x1fshell\x1fleft\x1fzsh\x1f0\x1f120\x1f40\x1f1\x1frelay\x1fcodex\x1fold\x1fmanual-attach\x1f1700000000",
				"%507\x1fdev\x1f@1\x1fshell\x1fright\x1fcodex\x1f0\x1f120\x1f40\x1f1\x1frelay\x1fcodex\x1fnew\x1fmanual-attach\x1f1700000001",
			}, "\n") + "\n"), nil
		},
	}
	client := NewClient(runner, "")

	state, err := client.GetPaneState(context.Background(), Target{PaneID: "%507"})
	if err != nil {
		t.Fatalf("GetPaneState() error = %v", err)
	}
	if state.Info.Target.PaneID != "%507" || state.Metadata.Label != "new" {
		t.Fatalf("unexpected state %#v", state)
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
	if got := runner.calls[0].args; len(got) < 4 || got[0] != "load-buffer" || got[1] != "-b" || !strings.HasPrefix(got[2], "tagb-5-") || got[3] != "-" {
		t.Fatalf("unexpected load-buffer args: %v", got)
	}
	if got := runner.calls[1].args; len(got) < 7 || got[0] != "paste-buffer" || got[1] != "-b" || !strings.HasPrefix(got[2], "tagb-5-") {
		t.Fatalf("unexpected paste-buffer args: %v", got)
	}
	if runner.calls[0].args[2] != runner.calls[1].args[2] {
		t.Fatalf("expected load-buffer and paste-buffer to use same buffer name: %v vs %v", runner.calls[0].args, runner.calls[1].args)
	}
}

func TestInjectInputUsesUniqueBufferNamesAcrossCalls(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	client := NewClient(runner, "")

	if err := client.InjectInput(context.Background(), Target{PaneID: "%5"}, []byte("hello")); err != nil {
		t.Fatalf("first InjectInput() error = %v", err)
	}
	if err := client.InjectInput(context.Background(), Target{PaneID: "%5"}, []byte("world")); err != nil {
		t.Fatalf("second InjectInput() error = %v", err)
	}
	if len(runner.calls) != 4 {
		t.Fatalf("expected 4 tmux calls, got %d", len(runner.calls))
	}
	first := runner.calls[0].args[2]
	second := runner.calls[2].args[2]
	if first == second {
		t.Fatalf("expected unique buffer names, got %q", first)
	}
}

func TestTouchMetadataUpdatesLastActivityOnlyForManagedPanes(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		runFn: func(_ context.Context, _ []byte, args ...string) (RunResult, error) {
			switch args[0] {
			case "show-options":
				return stdoutResult("1\n"), nil
			case "set-option":
				return RunResult{}, nil
			default:
				t.Fatalf("unexpected command: %v", args)
				return RunResult{}, nil
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

func TestSetMetadataUsesSingleTmuxCommand(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	client := NewClient(runner, "")

	meta := BridgeMetadata{
		Managed:          true,
		Mode:             ModeRelay,
		Agent:            AgentCodex,
		Label:            "backend",
		CreatedBy:        CreatedByManualAttach,
		LastActivityUnix: 1700000000,
	}
	if err := client.SetMetadata(context.Background(), Target{PaneID: "%5"}, meta); err != nil {
		t.Fatalf("SetMetadata() error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 tmux call, got %d", len(runner.calls))
	}
	got := runner.calls[0].args
	joined := strings.Join(got, " ")
	for _, fragment := range []string{
		"set-option -p -t %5 @tagb_managed 1",
		"set-option -p -t %5 @tagb_mode relay",
		"set-option -p -t %5 @tagb_agent codex",
		"set-option -p -t %5 @tagb_label backend",
		"set-option -p -t %5 @tagb_created_by manual-attach",
		"set-option -p -t %5 @tagb_last_activity_unix 1700000000",
	} {
		if !strings.Contains(joined, fragment) {
			t.Fatalf("missing fragment %q in args %v", fragment, got)
		}
	}
	if strings.Count(joined, " ; ") != 5 {
		t.Fatalf("expected 5 tmux separators in args %v", got)
	}
}

func TestClearMetadataUsesSingleTmuxCommand(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	client := NewClient(runner, "")

	if err := client.ClearMetadata(context.Background(), Target{PaneID: "%5"}); err != nil {
		t.Fatalf("ClearMetadata() error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 tmux call, got %d", len(runner.calls))
	}
	joined := strings.Join(runner.calls[0].args, " ")
	for _, key := range []string{OptionManaged, OptionMode, OptionAgent, OptionLabel, OptionCreatedBy, OptionLastActivity} {
		want := fmt.Sprintf("set-option -p -u -t %%5 %s", key)
		if !strings.Contains(joined, want) {
			t.Fatalf("missing unset command %q in args %v", want, runner.calls[0].args)
		}
	}
}

func TestDeleteUserOptionIgnoresUnsetErrors(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		runFn: func(_ context.Context, _ []byte, _ ...string) (RunResult, error) {
			return RunResult{}, errors.New("exit status 1: unknown option")
		},
	}
	client := NewClient(runner, "")

	if err := client.DeleteUserOption(context.Background(), Target{PaneID: "%1"}, OptionManaged); err != nil {
		t.Fatalf("DeleteUserOption() error = %v", err)
	}
}

func TestClassifyOptionErrorMarksUnavailableOptions(t *testing.T) {
	t.Parallel()

	for _, err := range []error{
		errors.New("exit status 1: unknown option"),
		errors.New("exit status 1: invalid option"),
		&TmuxCommandError{
			Result: RunResult{Stderr: "unknown option", ExitCode: 1},
			Err:    errors.New("exit status 1"),
		},
	} {
		if !errors.Is(classifyOptionError(err), ErrTmuxOptionUnavailable) {
			t.Fatalf("classifyOptionError(%v) should mark option unavailable", err)
		}
	}
}

func TestStartPollingSubscriptionHonorsLinesAndClose(t *testing.T) {
	t.Parallel()

	captured := make(chan []string, 1)
	runner := &fakeRunner{
		runFn: func(_ context.Context, _ []byte, args ...string) (RunResult, error) {
			if args[0] != "capture-pane" {
				t.Fatalf("unexpected command: %v", args)
			}
			captured <- append([]string(nil), args...)
			return stdoutResult("one\ntwo"), nil
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
		runFn: func(_ context.Context, _ []byte, args ...string) (RunResult, error) {
			if args[0] != "capture-pane" {
				t.Fatalf("unexpected command: %v", args)
			}
			if captureIndex >= len(captures) {
				return stdoutResult(captures[len(captures)-1]), nil
			}
			body := captures[captureIndex]
			captureIndex++
			return stdoutResult(body), nil
		},
		startPTYFn: func(context.Context, ...string) (PTYSession, error) {
			return nil, fmt.Errorf("%w: disabled for test", ErrControlUnsupported)
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

func TestOpenPaneStreamFallsBackToPollingOnControlErrors(t *testing.T) {
	t.Parallel()

	captures := []string{"one", "one\ntwo"}
	var captureIndex int
	runner := &fakeRunner{
		runFn: func(_ context.Context, _ []byte, args ...string) (RunResult, error) {
			if args[0] != "capture-pane" {
				t.Fatalf("unexpected command: %v", args)
			}
			if captureIndex >= len(captures) {
				return stdoutResult(captures[len(captures)-1]), nil
			}
			body := captures[captureIndex]
			captureIndex++
			return stdoutResult(body), nil
		},
		startPTYFn: func(context.Context, ...string) (PTYSession, error) {
			return nil, errors.New("control setup broke")
		},
	}
	client := NewClient(runner, "")
	initial, sub, err := client.OpenPaneStream(context.Background(), PaneInfo{
		Target:      Target{PaneID: "%9"},
		SessionName: "dev",
	}, 20)
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

func TestStartControlSubscriptionUsesSessionScopedPaneListAndWaitsForExitOnClose(t *testing.T) {
	t.Parallel()

	var startCtx context.Context
	session := &fakePTYSession{name: "/tmp/fake-tty"}
	runner := &fakeRunner{
		runFn: func(_ context.Context, _ []byte, args ...string) (RunResult, error) {
			switch args[0] {
			case "list-clients":
				return stdoutResult("/tmp/fake-tty\t1\n"), nil
			case "list-panes":
				if len(args) < 4 || args[1] != "-t" || args[2] != "dev" {
					t.Fatalf("expected session-scoped list-panes, got %v", args)
				}
				return stdoutResult("%9\x1fdev\x1f@1\x1fshell\x1fapi\x1fzsh\x1f0\x1f120\x1f40\n%10\x1fdev\x1f@1\x1fshell\x1fapi\x1fzsh\x1f0\x1f120\x1f40\n"), nil
			case "refresh-client":
				return RunResult{}, nil
			case "detach-client":
				return RunResult{}, nil
			default:
				t.Fatalf("unexpected command: %v", args)
				return RunResult{}, nil
			}
		},
		startPTYFn: func(ctx context.Context, args ...string) (PTYSession, error) {
			startCtx = ctx
			if got := strings.Join(args, " "); !strings.Contains(got, "attach-session -t dev -f ignore-size,active-pane") {
				t.Fatalf("unexpected StartPTY args: %v", args)
			}
			return session, nil
		},
	}
	client := NewClient(runner, "")
	sub, err := client.startControlSubscription(context.Background(), PaneInfo{
		Target:      Target{PaneID: "%9"},
		SessionName: "dev",
	})
	if err != nil {
		t.Fatalf("startControlSubscription() error = %v", err)
	}
	if err := sub.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if startCtx == nil {
		t.Fatal("expected StartPTY context")
	}
	select {
	case <-startCtx.Done():
	default:
		t.Fatal("expected StartPTY context to be canceled on Close")
	}
	if session.waitHits.Load() != 1 {
		t.Fatalf("expected Wait() to be called once, got %d", session.waitHits.Load())
	}
	if session.closeHits.Load() != 1 {
		t.Fatalf("expected Close() to be called once, got %d", session.closeHits.Load())
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

func TestCapturePaneRichCachesUnsupportedCapability(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		runFn: func(_ context.Context, _ []byte, _ ...string) (RunResult, error) {
			return RunResult{}, errors.New("invalid option")
		},
	}
	client := NewClient(runner, "")

	if _, err := client.CapturePaneRich(context.Background(), Target{PaneID: "%5"}, 20); !errors.Is(err, ErrRichCaptureUnsupported) {
		t.Fatalf("CapturePaneRich() error = %v, want ErrRichCaptureUnsupported", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 tmux call after first unsupported probe, got %d", len(runner.calls))
	}
	if _, err := client.CapturePaneRich(context.Background(), Target{PaneID: "%5"}, 20); !errors.Is(err, ErrRichCaptureUnsupported) {
		t.Fatalf("second CapturePaneRich() error = %v, want ErrRichCaptureUnsupported", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected cached unsupported capability to skip extra tmux calls, got %d", len(runner.calls))
	}
}

func TestStartControlSubscriptionCachesUnsupportedCapability(t *testing.T) {
	t.Parallel()

	var startCalls int
	runner := &fakeRunner{
		startPTYFn: func(context.Context, ...string) (PTYSession, error) {
			startCalls++
			return nil, errors.New("unknown option")
		},
	}
	client := NewClient(runner, "")
	pane := PaneInfo{Target: Target{PaneID: "%9"}, SessionName: "dev"}

	if _, err := client.startControlSubscription(context.Background(), pane); !errors.Is(err, ErrControlUnsupported) {
		t.Fatalf("startControlSubscription() error = %v, want ErrControlUnsupported", err)
	}
	if _, err := client.startControlSubscription(context.Background(), pane); !errors.Is(err, ErrControlUnsupported) {
		t.Fatalf("second startControlSubscription() error = %v, want ErrControlUnsupported", err)
	}
	if startCalls != 1 {
		t.Fatalf("expected control unsupported capability to skip extra StartPTY calls, got %d", startCalls)
	}
}
