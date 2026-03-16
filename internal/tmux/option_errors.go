package tmux

import (
	"errors"
	"strings"
)

var ErrTmuxOptionUnavailable = errors.New("tmux option unavailable")

func classifyOptionError(err error) error {
	if err == nil || errors.Is(err, ErrTmuxOptionUnavailable) {
		return err
	}
	if cmdErr := (*TmuxCommandError)(nil); errors.As(err, &cmdErr) {
		msg := strings.ToLower(cmdErr.Result.Stderr)
		if strings.Contains(msg, "invalid option") || strings.Contains(msg, "unknown option") {
			return errors.Join(ErrTmuxOptionUnavailable, err)
		}
		if cmdErr.Result.ExitCode == 1 {
			return errors.Join(ErrTmuxOptionUnavailable, err)
		}
		return err
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "invalid option") || strings.Contains(msg, "unknown option") {
		return errors.Join(ErrTmuxOptionUnavailable, err)
	}
	return err
}
