package tagb

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/portgle/tmux-connect/internal/tmux"
)

type fakeRunner struct {
	runFn func(context.Context, []byte, ...string) (tmux.RunResult, error)
}

func (r *fakeRunner) Run(ctx context.Context, stdin []byte, args ...string) (tmux.RunResult, error) {
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
