package tmux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/creack/pty"
)

const paneFieldSep = "\x1f"
const paneFieldSepEscaped = `\037`

type PTYSession interface {
	io.ReadWriteCloser
	Name() string
	Wait() error
}

func (RealRunner) StartPTY(ctx context.Context, args ...string) (PTYSession, error) {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	return &realPTYSession{file: ptmx, cmd: cmd}, nil
}

type realPTYSession struct {
	file *os.File
	cmd  *exec.Cmd
}

func (s *realPTYSession) Read(p []byte) (int, error)  { return s.file.Read(p) }
func (s *realPTYSession) Write(p []byte) (int, error) { return s.file.Write(p) }
func (s *realPTYSession) Close() error                { return s.file.Close() }
func (s *realPTYSession) Name() string                { return s.file.Name() }
func (s *realPTYSession) Wait() error                 { return s.cmd.Wait() }

type Client struct {
	runner       Runner
	socket       string
	capabilities capabilityCache
}

var injectBufferSeq atomic.Uint64

type PasteMode string

const (
	PasteModeBracketed PasteMode = "bracketed"
	PasteModePlain     PasteMode = "plain"
)

func NewClient(runner Runner, socket string) *Client {
	return &Client{runner: runner, socket: socket}
}

func (c *Client) SocketName() string {
	return normalizedSocket(c.socket)
}

func (c *Client) InjectInput(ctx context.Context, target Target, data []byte) error {
	return c.InjectInputWithMode(ctx, target, data, PasteModeBracketed)
}

func (c *Client) InjectInputWithMode(ctx context.Context, target Target, data []byte, mode PasteMode) error {
	if len(data) == 0 {
		return nil
	}
	bufferName := fmt.Sprintf("tmuxconn-%s-%d", strings.TrimPrefix(target.PaneID, "%"), injectBufferSeq.Add(1))
	if _, err := c.run(ctx, data, "load-buffer", "-b", bufferName, "-"); err != nil {
		return err
	}
	args := []string{"paste-buffer", "-b", bufferName, "-d"}
	if mode != PasteModePlain {
		args = append(args, "-p")
	}
	args = append(args, "-t", target.PaneID)
	_, err := c.run(ctx, nil, args...)
	return err
}

func (c *Client) SendKeys(ctx context.Context, target Target, keys ...string) error {
	args := append([]string{"send-keys", "-t", target.PaneID}, keys...)
	_, err := c.run(ctx, nil, args...)
	return err
}

func (c *Client) SubscribePane(ctx context.Context, pane PaneInfo, lines int) (*Subscription, error) {
	control, err := c.startControlSubscription(ctx, pane)
	if err == nil {
		return control, nil
	}
	if !shouldFallbackToPolling(err) {
		return nil, err
	}
	initial, captureErr := c.CapturePane(ctx, pane.Target, lines)
	if captureErr != nil {
		return nil, captureErr
	}
	return c.startPollingSubscriptionWithBaseline(ctx, pane, lines, initial), nil
}

func (c *Client) OpenPaneStream(ctx context.Context, pane PaneInfo, lines int) (string, *Subscription, error) {
	control, err := c.startControlSubscription(ctx, pane)
	if err == nil {
		initial, captureErr := c.CapturePane(ctx, pane.Target, lines)
		if captureErr != nil {
			_ = control.Close()
			return "", nil, captureErr
		}
		stream, cutoverErr := CutoverSubscription(control, initial)
		if cutoverErr != nil {
			return "", nil, cutoverErr
		}
		return initial, stream, nil
	}
	if !shouldFallbackToPolling(err) {
		return "", nil, err
	}

	initial, captureErr := c.CapturePane(ctx, pane.Target, lines)
	if captureErr != nil {
		return "", nil, captureErr
	}
	return initial, c.startPollingSubscriptionWithBaseline(ctx, pane, lines, initial), nil
}

func (c *Client) run(ctx context.Context, stdin []byte, args ...string) (string, error) {
	fullArgs := make([]string, 0, len(args)+2)
	if socket := strings.TrimSpace(c.socket); socket != "" {
		fullArgs = append(fullArgs, "-L", socket)
	}
	fullArgs = append(fullArgs, args...)
	result, err := c.runner.Run(ctx, stdin, fullArgs...)
	return result.Stdout, err
}

func isClosedConn(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || errors.Is(err, syscall.EIO)
}

func shouldFallbackToPolling(err error) bool {
	if err == nil {
		return false
	}
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}
