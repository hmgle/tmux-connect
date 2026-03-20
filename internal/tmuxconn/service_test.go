package tmuxconn

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/hmgle/tmux-connect/internal/tmux"
)

type fakeRunner struct {
	runFn func(context.Context, []byte, ...string) (tmux.RunResult, error)
	calls []runnerCall
}

type runnerCall struct {
	stdin []byte
	args  []string
}

func (r *fakeRunner) Run(ctx context.Context, stdin []byte, args ...string) (tmux.RunResult, error) {
	r.calls = append(r.calls, runnerCall{
		stdin: append([]byte(nil), stdin...),
		args:  append([]string(nil), args...),
	})
	if r.runFn != nil {
		return r.runFn(ctx, stdin, args...)
	}
	return tmux.RunResult{}, nil
}

func (r *fakeRunner) StartPTY(context.Context, ...string) (tmux.PTYSession, error) {
	return nil, errors.New("not implemented")
}

type fakePTYSession struct{}

func (s *fakePTYSession) Read([]byte) (int, error)    { return 0, io.EOF }
func (s *fakePTYSession) Write(p []byte) (int, error) { return len(p), nil }
func (s *fakePTYSession) Close() error                { return nil }
func (s *fakePTYSession) Name() string                { return "" }
func (s *fakePTYSession) Wait() error                 { return nil }

func TestResolvePanePreservesTmuxFailures(t *testing.T) {
	t.Parallel()

	runErr := &tmux.TmuxCommandError{
		Result: tmux.RunResult{
			Stderr:   "failed to connect to tmux server",
			ExitCode: 1,
		},
		Err: errors.New("exit status 1"),
	}
	service := NewService(tmux.NewClient(&fakeRunner{
		runFn: func(_ context.Context, _ []byte, args ...string) (tmux.RunResult, error) {
			if len(args) < 3 || args[0] != "list-panes" || args[1] != "-t" || args[2] != "%5" {
				t.Fatalf("unexpected args: %v", args)
			}
			return tmux.RunResult{Stderr: "failed to connect to tmux server", ExitCode: 1}, runErr
		},
	}, ""))

	_, err := service.ResolvePane(context.Background(), "%5")
	if ExitCode(err) != ExitTmuxFailure {
		t.Fatalf("ExitCode(error) = %d, want %d (err=%v)", ExitCode(err), ExitTmuxFailure, err)
	}
	if !strings.Contains(err.Error(), "failed to connect to tmux server") {
		t.Fatalf("error = %v, want tmux failure details", err)
	}
}

func TestResolvePaneReturnsNotFoundForMissingPane(t *testing.T) {
	t.Parallel()

	service := NewService(tmux.NewClient(&fakeRunner{
		runFn: func(_ context.Context, _ []byte, args ...string) (tmux.RunResult, error) {
			if len(args) < 3 || args[0] != "list-panes" || args[1] != "-t" || args[2] != "%5" {
				t.Fatalf("unexpected args: %v", args)
			}
			return tmux.RunResult{}, nil
		},
	}, ""))

	_, err := service.ResolvePane(context.Background(), "%5")
	if ExitCode(err) != ExitNotFound {
		t.Fatalf("ExitCode(error) = %d, want %d (err=%v)", ExitCode(err), ExitNotFound, err)
	}
}

func TestInspectPreservesTmuxFailures(t *testing.T) {
	t.Parallel()

	runErr := &tmux.TmuxCommandError{
		Result: tmux.RunResult{
			Stderr:   "lost connection to tmux",
			ExitCode: 1,
		},
		Err: errors.New("exit status 1"),
	}
	service := NewService(tmux.NewClient(&fakeRunner{
		runFn: func(_ context.Context, _ []byte, args ...string) (tmux.RunResult, error) {
			if len(args) < 3 || args[0] != "list-panes" || args[1] != "-t" || args[2] != "%5" {
				t.Fatalf("unexpected args: %v", args)
			}
			return tmux.RunResult{Stderr: "lost connection to tmux", ExitCode: 1}, runErr
		},
	}, ""))

	_, err := service.Inspect(context.Background(), "%5")
	if ExitCode(err) != ExitTmuxFailure {
		t.Fatalf("ExitCode(error) = %d, want %d (err=%v)", ExitCode(err), ExitTmuxFailure, err)
	}
	if !strings.Contains(err.Error(), "lost connection to tmux") {
		t.Fatalf("error = %v, want tmux failure details", err)
	}
}

func TestSendManagedInjectsTextAndTouchesMetadata(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		runFn: func(_ context.Context, _ []byte, args ...string) (tmux.RunResult, error) {
			switch args[0] {
			case "list-panes":
				return tmux.RunResult{Stdout: "%5\x1fdev\x1f@1\x1fshell\x1fapi\x1fzsh\x1f/home/gle/project\x1f0\x1f120\x1f40\n"}, nil
			case "load-buffer", "paste-buffer", "set-option":
				return tmux.RunResult{}, nil
			default:
				t.Fatalf("unexpected command: %v", args)
				return tmux.RunResult{}, nil
			}
		},
	}
	service := NewService(tmux.NewClient(runner, ""))

	if err := service.SendManaged(context.Background(), "%5", "status --short", false); err != nil {
		t.Fatalf("SendManaged() error = %v", err)
	}
	if len(runner.calls) != 4 {
		t.Fatalf("calls = %d, want 4", len(runner.calls))
	}
	if got := runner.calls[1].args; len(got) < 4 || got[0] != "load-buffer" || got[1] != "-b" || got[3] != "-" {
		t.Fatalf("load-buffer args = %v, want tmux buffer load", got)
	}
	if string(runner.calls[1].stdin) != "status --short" {
		t.Fatalf("load-buffer stdin = %q, want input text", string(runner.calls[1].stdin))
	}
	if got := runner.calls[2].args; len(got) < 7 || got[0] != "paste-buffer" || got[1] != "-b" || got[3] != "-d" || got[4] != "-p" || got[5] != "-t" || got[6] != "%5" {
		t.Fatalf("paste-buffer args = %v, want paste into pane", got)
	}
	if got := runner.calls[3].args; len(got) != 6 || got[0] != "set-option" || got[4] != tmux.OptionLastActivity {
		t.Fatalf("set-option args = %v, want metadata touch", got)
	}
}

func TestSendKeysManagedUsesSendKeysAndTouchesMetadata(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		runFn: func(_ context.Context, _ []byte, args ...string) (tmux.RunResult, error) {
			switch args[0] {
			case "list-panes":
				return tmux.RunResult{Stdout: "%5\x1fdev\x1f@1\x1fshell\x1fapi\x1fzsh\x1f/home/gle/project\x1f0\x1f120\x1f40\n"}, nil
			case "send-keys", "set-option":
				return tmux.RunResult{}, nil
			default:
				t.Fatalf("unexpected command: %v", args)
				return tmux.RunResult{}, nil
			}
		},
	}
	service := NewService(tmux.NewClient(runner, ""))

	if err := service.SendKeysManaged(context.Background(), "%5", "C-c", "Enter"); err != nil {
		t.Fatalf("SendKeysManaged() error = %v", err)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("calls = %d, want 3", len(runner.calls))
	}
	if got := runner.calls[1].args; len(got) != 5 || got[0] != "send-keys" || got[1] != "-t" || got[2] != "%5" || got[3] != "C-c" || got[4] != "Enter" {
		t.Fatalf("send-keys args = %v, want key send", got)
	}
	if got := runner.calls[2].args; len(got) != 6 || got[0] != "set-option" || got[4] != tmux.OptionLastActivity {
		t.Fatalf("set-option args = %v, want metadata touch", got)
	}
}
