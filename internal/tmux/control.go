package tmux

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
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

func (c *Client) startControlSubscription(ctx context.Context, pane PaneInfo) (*Subscription, error) {
	ptySession, err := c.runner.StartPTY(ctx, c.withSocket("attach-session", "-t", pane.SessionName, "-f", "ignore-size,active-pane")...)
	if err != nil {
		return nil, err
	}
	ttyName := ptySession.Name()
	if err := c.waitForControlClient(ctx, ttyName); err != nil {
		_ = ptySession.Close()
		return nil, err
	}

	panes, err := c.ListPanes(ctx)
	if err != nil {
		_ = ptySession.Close()
		return nil, err
	}
	if err := c.runClientCommand(ctx, ttyName, "refresh-client", "-C", "1x1"); err != nil {
		_ = ptySession.Close()
		return nil, err
	}
	for _, candidate := range panes {
		if candidate.SessionName != pane.SessionName {
			continue
		}
		state := "off"
		if candidate.Target.PaneID == pane.Target.PaneID {
			state = "on"
		}
		if err := c.runClientCommand(ctx, ttyName, "refresh-client", "-A", candidate.Target.PaneID+":"+state); err != nil {
			_ = ptySession.Close()
			return nil, err
		}
	}

	sub := &Subscription{
		chunks: make(chan OutputChunk, 16),
		errs:   make(chan error, 1),
	}
	sub.close = func() error {
		_, _ = c.run(context.Background(), nil, "detach-client", "-t", ttyName)
		return ptySession.Close()
	}

	go c.readControlOutput(ctx, pane.Target, ptySession, sub)
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
		if chunk, ok := parseNotification(target, line); ok {
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
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("control client did not register")
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

func parseNotification(target Target, line string) (OutputChunk, bool) {
	if strings.HasPrefix(line, "%output ") {
		parts := strings.SplitN(line, " ", 3)
		if len(parts) != 3 || parts[1] != target.PaneID {
			return OutputChunk{}, false
		}
		text, err := decodeTmuxEscapes(parts[2])
		if err != nil {
			return OutputChunk{}, false
		}
		return OutputChunk{Target: target, Text: text, ReceivedAt: time.Now()}, true
	}
	if strings.HasPrefix(line, "%extended-output ") {
		parts := strings.SplitN(line, " ", 4)
		if len(parts) < 4 || parts[1] != target.PaneID {
			return OutputChunk{}, false
		}
		index := strings.Index(line, ": ")
		if index == -1 {
			return OutputChunk{}, false
		}
		text, err := decodeTmuxEscapes(line[index+2:])
		if err != nil {
			return OutputChunk{}, false
		}
		return OutputChunk{Target: target, Text: text, ReceivedAt: time.Now()}, true
	}
	return OutputChunk{}, false
}

func cleanControlLine(line string) string {
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	line = strings.ReplaceAll(line, "\x1bP1000p", "")
	line = strings.ReplaceAll(line, "\x1b\\", "")
	return strings.TrimSpace(line)
}

func decodeTmuxEscapes(value string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if ch != '\\' {
			b.WriteByte(ch)
			continue
		}
		if i+3 >= len(value) {
			return "", fmt.Errorf("invalid tmux escape")
		}
		octal := value[i+1 : i+4]
		var decoded byte
		for j := 0; j < 3; j++ {
			if octal[j] < '0' || octal[j] > '7' {
				return "", fmt.Errorf("invalid tmux escape %q", octal)
			}
			decoded = decoded*8 + (octal[j] - '0')
		}
		b.WriteByte(decoded)
		i += 3
	}
	return b.String(), nil
}
