package tmux

import (
	"errors"
	"strings"
	"sync/atomic"
)

var ErrRichCaptureUnsupported = errors.New("tmux rich capture unsupported")

type capabilityCache struct {
	controlUnsupported     atomic.Bool
	richCaptureUnsupported atomic.Bool
}

func (c *Client) supportsControlMode() bool {
	return !c.capabilities.controlUnsupported.Load()
}

func (c *Client) supportsRichCapture() bool {
	return !c.capabilities.richCaptureUnsupported.Load()
}

func (c *Client) markControlModeUnsupported() {
	c.capabilities.controlUnsupported.Store(true)
}

func (c *Client) markRichCaptureUnsupported() {
	c.capabilities.richCaptureUnsupported.Store(true)
}

func isUnsupportedFeatureError(err error) bool {
	if err == nil {
		return false
	}
	if cmdErr := (*TmuxCommandError)(nil); errors.As(err, &cmdErr) {
		msg := strings.ToLower(cmdErr.Result.Stderr)
		for _, marker := range []string{
			"unknown option",
			"unknown flag",
			"unknown format",
			"bad flag",
			"invalid option",
			"invalid flag",
		} {
			if strings.Contains(msg, marker) {
				return true
			}
		}
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"unknown option",
		"unknown flag",
		"unknown format",
		"bad flag",
		"invalid option",
		"invalid flag",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}
