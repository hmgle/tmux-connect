package tmux

import (
	"errors"
	"os/exec"
	"strings"
)

var ErrTmuxOptionUnavailable = errors.New("tmux option unavailable")

func classifyOptionError(err error) error {
	if err == nil || errors.Is(err, ErrTmuxOptionUnavailable) {
		return err
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "invalid option") || strings.Contains(msg, "unknown option") {
		return errors.Join(ErrTmuxOptionUnavailable, err)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return errors.Join(ErrTmuxOptionUnavailable, err)
	}
	return err
}
