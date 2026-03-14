package tmux

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
)

type Runner interface {
	Run(ctx context.Context, stdin []byte, args ...string) (string, error)
	StartPTY(ctx context.Context, args ...string) (PTYSession, error)
}

type PTYSession interface {
	io.ReadWriteCloser
	Name() string
	Wait() error
}

type RealRunner struct{}

func (RealRunner) Run(ctx context.Context, stdin []byte, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
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
	runner Runner
	socket string
}

func NewClient(runner Runner, socket string) *Client {
	return &Client{runner: runner, socket: socket}
}

func (c *Client) SocketName() string {
	return normalizedSocket(c.socket)
}

func (c *Client) ListPanes(ctx context.Context) ([]PaneInfo, error) {
	format := "#{pane_id}\t#{session_name}\t#{window_id}\t#{window_name}\t#{pane_title}\t#{pane_current_command}\t#{pane_dead}\t#{pane_width}\t#{pane_height}"
	output, err := c.run(ctx, nil, "list-panes", "-a", "-F", format)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	panes := make([]PaneInfo, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 9 {
			return nil, fmt.Errorf("unexpected list-panes row: %q", line)
		}
		width, err := strconv.Atoi(fields[7])
		if err != nil {
			return nil, fmt.Errorf("parse width for %s: %w", fields[0], err)
		}
		height, err := strconv.Atoi(fields[8])
		if err != nil {
			return nil, fmt.Errorf("parse height for %s: %w", fields[0], err)
		}
		panes = append(panes, PaneInfo{
			Target:      Target{Socket: c.SocketName(), PaneID: fields[0]},
			SessionName: fields[1],
			WindowID:    fields[2],
			WindowName:  fields[3],
			PaneTitle:   fields[4],
			CurrentCmd:  fields[5],
			Dead:        fields[6] == "1",
			Width:       width,
			Height:      height,
		})
	}
	return panes, nil
}

func (c *Client) CapturePane(ctx context.Context, target Target, lines int) (string, error) {
	if lines <= 0 {
		lines = 120
	}
	start := strconv.Itoa(-(lines - 1))
	return c.run(ctx, nil, "capture-pane", "-p", "-J", "-t", target.PaneID, "-S", start)
}

func (c *Client) InjectInput(ctx context.Context, target Target, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if _, err := c.run(ctx, data, "load-buffer", "-"); err != nil {
		return err
	}
	_, err := c.run(ctx, nil, "paste-buffer", "-d", "-p", "-t", target.PaneID)
	return err
}

func (c *Client) SendKeys(ctx context.Context, target Target, keys ...string) error {
	args := append([]string{"send-keys", "-t", target.PaneID}, keys...)
	_, err := c.run(ctx, nil, args...)
	return err
}

func (c *Client) GetUserOptions(ctx context.Context, target Target) (map[string]string, error) {
	output, err := c.run(ctx, nil, "show-options", "-p", "-t", target.PaneID)
	if err != nil {
		if strings.Contains(err.Error(), "invalid option") {
			return map[string]string{}, nil
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return map[string]string{}, nil
		}
		return nil, err
	}
	return parseUserOptions(output), nil
}

func (c *Client) SetUserOption(ctx context.Context, target Target, key string, value string) error {
	_, err := c.run(ctx, nil, "set-option", "-p", "-t", target.PaneID, key, value)
	return err
}

func (c *Client) DeleteUserOption(ctx context.Context, target Target, key string) error {
	_, err := c.run(ctx, nil, "set-option", "-p", "-u", "-t", target.PaneID, key)
	return err
}

func (c *Client) GetMetadata(ctx context.Context, target Target) (BridgeMetadata, error) {
	opts, err := c.GetUserOptions(ctx, target)
	if err != nil {
		return BridgeMetadata{}, err
	}
	return MetadataFromOptions(opts), nil
}

func (c *Client) SetMetadata(ctx context.Context, target Target, meta BridgeMetadata) error {
	for key, value := range meta.ToOptions() {
		if err := c.SetUserOption(ctx, target, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) ClearMetadata(ctx context.Context, target Target) error {
	for _, key := range []string{OptionManaged, OptionMode, OptionAgent, OptionLabel, OptionCreatedBy, OptionLastActivity} {
		if err := c.DeleteUserOption(ctx, target, key); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) TouchMetadata(ctx context.Context, target Target) error {
	meta, err := c.GetMetadata(ctx, target)
	if err != nil {
		return err
	}
	if !meta.Managed {
		return nil
	}
	meta.LastActivityUnix = time.Now().Unix()
	return c.SetMetadata(ctx, target, meta)
}

func (c *Client) SubscribePane(ctx context.Context, pane PaneInfo) (*Subscription, error) {
	control, err := c.startControlSubscription(ctx, pane)
	if err == nil {
		return control, nil
	}
	return c.startPollingSubscription(ctx, pane), nil
}

func (c *Client) run(ctx context.Context, stdin []byte, args ...string) (string, error) {
	fullArgs := make([]string, 0, len(args)+2)
	if socket := strings.TrimSpace(c.socket); socket != "" {
		fullArgs = append(fullArgs, "-L", socket)
	}
	fullArgs = append(fullArgs, args...)
	return c.runner.Run(ctx, stdin, fullArgs...)
}

func parseUserOptions(output string) map[string]string {
	opts := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "@tagb_") {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 1 {
			opts[parts[0]] = ""
			continue
		}
		opts[parts[0]] = strings.TrimSpace(parts[1])
	}
	return opts
}

func isClosedConn(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || errors.Is(err, syscall.EIO)
}
