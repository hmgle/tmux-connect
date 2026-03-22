package tmux

import (
	"bufio"
	"context"
	"errors"
	"io"
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
