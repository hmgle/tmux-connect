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

type Subscription struct {
	chunks chan OutputChunk
	errs   chan error
	close  func() error
}

func (s *Subscription) Chunks() <-chan OutputChunk { return s.chunks }
func (s *Subscription) Errs() <-chan error         { return s.errs }
func (s *Subscription) Close() error               { return s.close() }

// CutoverSubscription trims the buffered prefix already reflected in the
// initial snapshot and forwards the remaining stream through a fresh wrapper.
func CutoverSubscription(base *Subscription, initial string) (*Subscription, error) {
	if base == nil {
		return nil, fmt.Errorf("subscription is required")
	}

	pending, chunks, errs, err := drainBufferedSubscription(base)
	if err != nil {
		_ = base.Close()
		return nil, err
	}
	pending = trimSnapshotOverlap(initial, pending)

	out := &Subscription{
		chunks: make(chan OutputChunk, maxInt(cap(base.chunks), len(pending))),
		errs:   make(chan error, cap(base.errs)),
		close:  base.Close,
	}
	for _, chunk := range pending {
		out.chunks <- chunk
	}
	if chunks == nil && errs == nil {
		close(out.chunks)
		close(out.errs)
		return out, nil
	}

	go func() {
		defer close(out.chunks)
		defer close(out.errs)

		for {
			if chunks == nil && errs == nil {
				return
			}
			select {
			case err, ok := <-errs:
				if !ok {
					errs = nil
					continue
				}
				if err == nil {
					continue
				}
				select {
				case out.errs <- err:
				default:
				}
			case chunk, ok := <-chunks:
				if !ok {
					chunks = nil
					continue
				}
				out.chunks <- chunk
			}
		}
	}()

	return out, nil
}

func drainBufferedSubscription(base *Subscription) ([]OutputChunk, <-chan OutputChunk, <-chan error, error) {
	chunks := base.chunks
	errs := base.errs
	var pending []OutputChunk
	for {
		progressed := false

		select {
		case err, ok := <-errs:
			progressed = true
			if !ok {
				errs = nil
				break
			}
			if err != nil {
				return nil, nil, nil, err
			}
		default:
		}

		select {
		case chunk, ok := <-chunks:
			progressed = true
			if !ok {
				chunks = nil
				break
			}
			pending = append(pending, chunk)
		default:
		}

		if chunks == nil && errs == nil {
			return pending, nil, nil, nil
		}
		if !progressed {
			return pending, chunks, errs, nil
		}
	}
}

func trimSnapshotOverlap(initial string, pending []OutputChunk) []OutputChunk {
	if initial == "" || len(pending) == 0 {
		return pending
	}
	var buffered strings.Builder
	for _, chunk := range pending {
		buffered.WriteString(chunk.Text)
	}
	trimBytes := suffixPrefixOverlap(initial, buffered.String())
	if trimBytes == 0 {
		return pending
	}
	return trimChunkPrefix(pending, trimBytes)
}

func suffixPrefixOverlap(snapshot string, buffered string) int {
	maxOverlap := len(snapshot)
	if len(buffered) < maxOverlap {
		maxOverlap = len(buffered)
	}
	for overlap := maxOverlap; overlap > 0; overlap-- {
		if snapshot[len(snapshot)-overlap:] == buffered[:overlap] {
			return overlap
		}
	}
	return 0
}

func trimChunkPrefix(chunks []OutputChunk, trimBytes int) []OutputChunk {
	if trimBytes <= 0 {
		return chunks
	}
	trimmed := make([]OutputChunk, 0, len(chunks))
	remaining := trimBytes
	for _, chunk := range chunks {
		text := chunk.Text
		if remaining >= len(text) {
			remaining -= len(text)
			continue
		}
		if remaining > 0 {
			chunk.Text = text[remaining:]
			remaining = 0
		}
		if chunk.Text != "" {
			trimmed = append(trimmed, chunk)
		}
	}
	return trimmed
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

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

func (c *Client) startControlSubscription(ctx context.Context, pane PaneInfo) (*Subscription, error) {
	subCtx, cancel := context.WithCancel(ctx)
	ptySession, err := c.runner.StartPTY(subCtx, c.withSocket("attach-session", "-t", pane.SessionName, "-f", "ignore-size,active-pane")...)
	if err != nil {
		cancel()
		return nil, classifyControlError(err)
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
		return nil, classifyControlError(err)
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
			return nil, classifyControlError(err)
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

func parseNotification(target Target, line string) (OutputChunk, bool, error) {
	if strings.HasPrefix(line, "%output ") {
		parts := strings.SplitN(line, " ", 3)
		if len(parts) != 3 {
			return OutputChunk{}, false, fmt.Errorf("malformed %%output notification: %q", line)
		}
		if parts[1] != target.PaneID {
			return OutputChunk{}, false, nil
		}
		text, err := decodeTmuxEscapes(parts[2])
		if err != nil {
			return OutputChunk{}, false, err
		}
		return OutputChunk{Target: target, Text: text, ReceivedAt: time.Now()}, true, nil
	}
	if strings.HasPrefix(line, "%extended-output ") {
		parts := strings.SplitN(line, " ", 4)
		if len(parts) < 4 {
			return OutputChunk{}, false, fmt.Errorf("malformed %%extended-output notification: %q", line)
		}
		if parts[1] != target.PaneID {
			return OutputChunk{}, false, nil
		}
		index := strings.Index(line, ": ")
		if index == -1 {
			return OutputChunk{}, false, fmt.Errorf("missing payload in %%extended-output notification: %q", line)
		}
		text, err := decodeTmuxEscapes(line[index+2:])
		if err != nil {
			return OutputChunk{}, false, err
		}
		return OutputChunk{Target: target, Text: text, ReceivedAt: time.Now()}, true, nil
	}
	return OutputChunk{}, false, nil
}

func cleanControlLine(line string) string {
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	line = strings.ReplaceAll(line, "\x1bP1000p", "")
	line = strings.ReplaceAll(line, "\x1b\\", "")
	return strings.TrimSpace(line)
}

func waitForPTYExit(waitDone <-chan error, timeout time.Duration) error {
	if waitDone == nil {
		return nil
	}
	select {
	case err, ok := <-waitDone:
		if !ok || err == nil {
			return nil
		}
		return err
	case <-time.After(timeout):
		return fmt.Errorf("%w: process did not exit within %s", ErrControlProtocol, timeout)
	}
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
	return strings.Contains(msg, "unknown option") ||
		strings.Contains(msg, "unknown command") ||
		strings.Contains(msg, "bad flag") ||
		strings.Contains(msg, "usage: refresh-client") ||
		strings.Contains(msg, "command attach-session")
}
