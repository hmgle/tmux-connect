package tmux

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
)

type RunnerCall struct {
	Stdin []byte
	Args  []string
}

type FakeRunner struct {
	RunFn      func(context.Context, []byte, ...string) (RunResult, error)
	StartPTYFn func(context.Context, ...string) (PTYSession, error)
	Calls      []RunnerCall
}

func StdoutResult(stdout string) RunResult {
	return RunResult{Stdout: stdout}
}

func (r *FakeRunner) Run(ctx context.Context, stdin []byte, args ...string) (RunResult, error) {
	r.Calls = append(r.Calls, RunnerCall{
		Stdin: append([]byte(nil), stdin...),
		Args:  append([]string(nil), args...),
	})
	if r.RunFn != nil {
		return r.RunFn(ctx, stdin, args...)
	}
	return RunResult{}, nil
}

func (r *FakeRunner) StartPTY(ctx context.Context, args ...string) (PTYSession, error) {
	if r.StartPTYFn != nil {
		return r.StartPTYFn(ctx, args...)
	}
	return nil, errors.New("not implemented")
}

type FakePTYSession struct {
	SessionName string
	CloseFn     func() error
	WaitFn      func() error
	CloseHits   atomic.Int32
	WaitHits    atomic.Int32
}

func (s *FakePTYSession) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (s *FakePTYSession) Write(p []byte) (int, error) { return len(p), nil }
func (s *FakePTYSession) Close() error {
	s.CloseHits.Add(1)
	if s.CloseFn != nil {
		return s.CloseFn()
	}
	return nil
}

func (s *FakePTYSession) Name() string { return s.SessionName }

func (s *FakePTYSession) Wait() error {
	s.WaitHits.Add(1)
	if s.WaitFn != nil {
		return s.WaitFn()
	}
	return nil
}

func NewSubscriptionForTest() *Subscription {
	return &Subscription{
		chunks: make(chan OutputChunk, 8),
		errs:   make(chan error, 1),
		close:  func() error { return nil },
	}
}

func (s *Subscription) PushChunk(chunk OutputChunk) {
	s.chunks <- chunk
}

func (s *Subscription) PushError(err error) {
	s.errs <- err
}

func (s *Subscription) CloseErrs() {
	close(s.errs)
}

func (s *Subscription) CloseChunks() {
	close(s.chunks)
}

func (s *Subscription) CloseChannels() {
	s.CloseChunks()
	s.CloseErrs()
}
