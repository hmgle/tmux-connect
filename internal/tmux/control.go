package tmux

import (
	"errors"
	"fmt"
	"time"
)

type Subscription struct {
	chunks chan OutputChunk
	errs   chan error
	close  func() error
}

func (s *Subscription) Chunks() <-chan OutputChunk { return s.chunks }
func (s *Subscription) Errs() <-chan error         { return s.errs }
func (s *Subscription) Close() error               { return s.close() }

var (
	ErrControlUnsupported      = errors.New("tmux control mode unsupported")
	ErrControlHandshakeTimeout = errors.New("tmux control handshake timeout")
	ErrControlProtocol         = errors.New("tmux control protocol error")
)

type controlModeError struct {
	kind error
	err  error
}

func (e *controlModeError) Error() string {
	if e == nil {
		return ""
	}
	if e.err == nil {
		return e.kind.Error()
	}
	return e.err.Error()
}

func (e *controlModeError) Unwrap() error { return e.err }

func (e *controlModeError) Is(target error) bool {
	return e != nil && target == e.kind
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func waitForPTYExit(waitDone <-chan error, timeoutDuration time.Duration) error {
	if waitDone == nil {
		return nil
	}
	select {
	case err, ok := <-waitDone:
		if !ok || err == nil {
			return nil
		}
		return err
	case <-time.After(timeoutDuration):
		return fmt.Errorf("%w: process did not exit within %s", ErrControlProtocol, timeoutDuration)
	}
}
