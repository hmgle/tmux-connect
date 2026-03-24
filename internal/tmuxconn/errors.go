package tmuxconn

import (
	"errors"
	"flag"
	"fmt"
)

const (
	ExitOK          = 0
	ExitUsage       = 2
	ExitNotFound    = 3
	ExitTmuxFailure = 4
)

type CLIError struct {
	Code int
	Err  error
}

func (e *CLIError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *CLIError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func UsageError(format string, args ...any) error {
	return &CLIError{Code: ExitUsage, Err: fmt.Errorf(format, args...)}
}

func NotFoundError(format string, args ...any) error {
	return &CLIError{Code: ExitNotFound, Err: fmt.Errorf(format, args...)}
}

func TmuxError(format string, args ...any) error {
	return &CLIError{Code: ExitTmuxFailure, Err: fmt.Errorf(format, args...)}
}

func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	if IsHelpError(err) {
		return ExitOK
	}
	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		return cliErr.Code
	}
	return 1
}

func IsHelpError(err error) bool {
	return errors.Is(err, flag.ErrHelp)
}
