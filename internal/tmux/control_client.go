package tmux

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

func (c *Client) startControlSubscription(ctx context.Context, pane PaneInfo) (*Subscription, error) {
	if !c.supportsControlMode() {
		return nil, ErrControlUnsupported
	}
	subCtx, cancel := context.WithCancel(ctx)
	ptySession, err := c.runner.StartPTY(subCtx, c.withSocket("attach-session", "-t", pane.SessionName, "-f", "ignore-size,active-pane")...)
	if err != nil {
		if classified := classifyControlError(err); errors.Is(classified, ErrControlUnsupported) {
			cancel()
			c.markControlModeUnsupported()
			return nil, classified
		}
		cancel()
		return nil, err
	}
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- ptySession.Wait()
		close(waitDone)
	}()
	ttyName := ptySession.Name()
	if err := c.waitForControlClient(subCtx, ttyName); err != nil {
		cancel()
		_ = ptySession.Close()
		<-waitDone
		return nil, err
	}

	panes, err := c.ListSessionPanes(subCtx, pane.SessionName)
	if err != nil {
		cancel()
		_ = ptySession.Close()
		<-waitDone
		return nil, err
	}
	if err := c.runClientCommand(subCtx, ttyName, "refresh-client", "-C", "1x1"); err != nil {
		cancel()
		_ = ptySession.Close()
		<-waitDone
		classified := classifyControlError(err)
		if errors.Is(classified, ErrControlUnsupported) {
			c.markControlModeUnsupported()
		}
		return nil, classified
	}
	for _, candidate := range panes {
		state := "off"
		if candidate.Target.PaneID == pane.Target.PaneID {
			state = "on"
		}
		if err := c.runClientCommand(subCtx, ttyName, "refresh-client", "-A", candidate.Target.PaneID+":"+state); err != nil {
			cancel()
			_ = ptySession.Close()
			<-waitDone
			classified := classifyControlError(err)
			if errors.Is(classified, ErrControlUnsupported) {
				c.markControlModeUnsupported()
			}
			return nil, classified
		}
	}

	sub := &Subscription{
		chunks: make(chan OutputChunk, 16),
		errs:   make(chan error, 1),
	}
	var (
		closeErr  error
		closeOnce sync.Once
	)
	sub.close = func() error {
		closeOnce.Do(func() {
			cancel()
			_, _ = c.run(context.Background(), nil, "detach-client", "-t", ttyName)
			closeErr = errors.Join(ptySession.Close(), waitForPTYExit(waitDone, 2*time.Second))
		})
		return closeErr
	}

	go c.readControlOutput(subCtx, pane.Target, ptySession, sub)
	return sub, nil
}

func (c *Client) readControlOutput(ctx context.Context, target Target, session PTYSession, sub *Subscription) {
	defer close(sub.chunks)
	defer close(sub.errs)

	reader := bufio.NewReader(session)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if !isClosedConn(err) && err != io.EOF {
				sub.errs <- err
			}
			return
		}
		line = cleanControlLine(line)
		if line == "" {
			continue
		}
		chunk, ok, parseErr := parseNotification(target, line)
		if parseErr != nil {
			select {
			case sub.errs <- &controlModeError{kind: ErrControlProtocol, err: parseErr}:
			default:
			}
			return
		}
		if ok {
			select {
			case <-ctx.Done():
				return
			case sub.chunks <- chunk:
			}
		}
	}
}

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
