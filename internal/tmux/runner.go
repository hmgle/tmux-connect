package tmux

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Args     []string
}

type TmuxCommandError struct {
	Result RunResult
	Err    error
}

func (e *TmuxCommandError) Error() string {
	if e == nil {
		return ""
	}
	stderr := strings.TrimSpace(e.Result.Stderr)
	if stderr == "" {
		stderr = strings.TrimSpace(e.Result.Stdout)
	}
	if stderr == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%v: %s", e.Err, stderr)
}

func (e *TmuxCommandError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type Runner interface {
	Run(ctx context.Context, stdin []byte, args ...string) (RunResult, error)
	StartPTY(ctx context.Context, args ...string) (PTYSession, error)
}

type RealRunner struct{}

func (RealRunner) Run(ctx context.Context, stdin []byte, args ...string) (RunResult, error) {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := RunResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
		Args:   append([]string(nil), args...),
	}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		return result, &TmuxCommandError{Result: result, Err: err}
	}
	return result, nil
}
