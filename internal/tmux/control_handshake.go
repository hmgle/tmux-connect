package tmux

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (c *Client) waitForControlClient(ctx context.Context, ttyName string) error {
	deadline := time.Now().Add(2 * time.Second)
	for {
		format := "#{client_tty}\t#{client_control_mode}"
		output, err := c.run(ctx, nil, "list-clients", "-F", format)
		if err == nil {
			scanner := bufio.NewScanner(strings.NewReader(output))
			for scanner.Scan() {
				fields := strings.Split(scanner.Text(), "\t")
				if len(fields) == 2 && fields[0] == ttyName && fields[1] == "1" {
					return nil
				}
			}
		} else if classified := classifyControlError(err); errors.Is(classified, ErrControlUnsupported) {
			c.markControlModeUnsupported()
			return classified
		}
		if time.Now().After(deadline) {
			return &controlModeError{
				kind: ErrControlHandshakeTimeout,
				err:  fmt.Errorf("control client %q did not register", ttyName),
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (c *Client) runClientCommand(ctx context.Context, ttyName string, args ...string) error {
	fullArgs := append(args, "-t", ttyName)
	_, err := c.run(ctx, nil, fullArgs...)
	return err
}

func (c *Client) withSocket(args ...string) []string {
	fullArgs := make([]string, 0, len(args)+2)
	if socket := strings.TrimSpace(c.socket); socket != "" {
		fullArgs = append(fullArgs, "-L", socket)
	}
	fullArgs = append(fullArgs, args...)
	return fullArgs
}

func classifyControlError(err error) error {
	if err == nil || errors.Is(err, ErrControlUnsupported) || errors.Is(err, ErrControlHandshakeTimeout) || errors.Is(err, ErrControlProtocol) {
		return err
	}
	if isControlUnsupportedError(err) {
		return &controlModeError{kind: ErrControlUnsupported, err: err}
	}
	return err
}

func isControlUnsupportedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return isUnsupportedFeatureError(err) ||
		strings.Contains(msg, "unknown command") ||
		strings.Contains(msg, "bad flag") ||
		strings.Contains(msg, "usage: refresh-client") ||
		strings.Contains(msg, "command attach-session")
}
