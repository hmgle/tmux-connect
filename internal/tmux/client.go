package tmux

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

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

func NewClient(runner Runner, socket string) *Client {
	return &Client{runner: runner, socket: socket}
}

func (c *Client) SocketName() string {
	return normalizedSocket(c.socket)
}

func (c *Client) ListPanes(ctx context.Context) ([]PaneInfo, error) {
	return c.listPanes(ctx, []string{"-a"})
}

func (c *Client) ListSessionPanes(ctx context.Context, sessionName string) ([]PaneInfo, error) {
	return c.listPanes(ctx, []string{"-t", sessionName})
}

func (c *Client) ListPaneStates(ctx context.Context) ([]PaneState, error) {
	return c.listPaneStates(ctx, []string{"-a"})
}

func (c *Client) GetPane(ctx context.Context, target Target) (PaneInfo, error) {
	panes, err := c.listPanes(ctx, []string{"-t", target.PaneID})
	if err != nil {
		return PaneInfo{}, err
	}
	for _, pane := range panes {
		if pane.Target.Matches(target) {
			return pane, nil
		}
	}
	return PaneInfo{}, fmt.Errorf("pane not found: %s", target.PaneID)
}

func (c *Client) GetPaneState(ctx context.Context, target Target) (PaneState, error) {
	states, err := c.listPaneStates(ctx, []string{"-t", target.PaneID})
	if err != nil {
		return PaneState{}, err
	}
	for _, state := range states {
		if state.Info.Target.Matches(target) {
			return state, nil
		}
	}
	return PaneState{}, fmt.Errorf("pane not found: %s", target.PaneID)
}

func (c *Client) CapturePane(ctx context.Context, target Target, lines int) (string, error) {
	return c.capturePane(ctx, target, lines, false)
}

func (c *Client) CapturePaneRich(ctx context.Context, target Target, lines int) (string, error) {
	if !c.supportsRichCapture() {
		return "", ErrRichCaptureUnsupported
	}
	return c.capturePane(ctx, target, lines, true)
}

func (c *Client) capturePane(ctx context.Context, target Target, lines int, escape bool) (string, error) {
	if lines <= 0 {
		lines = 120
	}
	start := strconv.Itoa(-(lines - 1))
	args := []string{"capture-pane", "-p", "-J"}
	if escape {
		args = append(args, "-e")
	}
	args = append(args, "-t", target.PaneID, "-S", start)
	output, err := c.run(ctx, nil, args...)
	if err != nil && escape && isUnsupportedFeatureError(err) {
		c.markRichCaptureUnsupported()
		return "", errors.Join(ErrRichCaptureUnsupported, err)
	}
	return output, err
}

func (c *Client) InjectInput(ctx context.Context, target Target, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	bufferName := fmt.Sprintf("tmuxconn-%s-%d", strings.TrimPrefix(target.PaneID, "%"), injectBufferSeq.Add(1))
	if _, err := c.run(ctx, data, "load-buffer", "-b", bufferName, "-"); err != nil {
		return err
	}
	_, err := c.run(ctx, nil, "paste-buffer", "-b", bufferName, "-d", "-p", "-t", target.PaneID)
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
		if errors.Is(classifyOptionError(err), ErrTmuxOptionUnavailable) {
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

func (c *Client) GetUserOption(ctx context.Context, target Target, key string) (string, error) {
	output, err := c.run(ctx, nil, "show-options", "-p", "-v", "-t", target.PaneID, key)
	if err != nil {
		if errors.Is(classifyOptionError(err), ErrTmuxOptionUnavailable) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (c *Client) DeleteUserOption(ctx context.Context, target Target, key string) error {
	_, err := c.run(ctx, nil, "set-option", "-p", "-u", "-t", target.PaneID, key)
	if errors.Is(classifyOptionError(err), ErrTmuxOptionUnavailable) {
		return nil
	}
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
	return c.runOptionCommands(ctx, target, false, metadataOptionPairs(meta))
}

func (c *Client) ClearMetadata(ctx context.Context, target Target) error {
	return c.runOptionCommands(ctx, target, true, metadataOptionPairs(BridgeMetadata{}))
}

func (c *Client) TouchMetadata(ctx context.Context, target Target) error {
	managed, err := c.GetUserOption(ctx, target, OptionManaged)
	if err != nil {
		return err
	}
	if managed != "1" {
		return nil
	}
	return c.TouchMetadataManaged(ctx, target)
}

func (c *Client) TouchMetadataManaged(ctx context.Context, target Target) error {
	return c.SetUserOption(ctx, target, OptionLastActivity, strconv.FormatInt(time.Now().Unix(), 10))
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

func (c *Client) listPanes(ctx context.Context, extraArgs []string) ([]PaneInfo, error) {
	args := []string{"list-panes"}
	args = append(args, extraArgs...)
	args = append(args, "-F", paneListFormat())
	output, err := c.run(ctx, nil, args...)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	panes := make([]PaneInfo, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		pane, parseErr := parsePaneInfoLine(c.SocketName(), line)
		if parseErr != nil {
			return nil, parseErr
		}
		panes = append(panes, pane)
	}
	return panes, nil
}

func (c *Client) listPaneStates(ctx context.Context, extraArgs []string) ([]PaneState, error) {
	args := []string{"list-panes"}
	args = append(args, extraArgs...)
	args = append(args, "-F", paneStateFormat())
	output, err := c.run(ctx, nil, args...)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	states := make([]PaneState, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		pane, meta, parseErr := parsePaneStateLine(c.SocketName(), line)
		if parseErr != nil {
			return nil, parseErr
		}
		states = append(states, PaneState{Info: pane, Metadata: meta})
	}
	return states, nil
}

func (c *Client) runOptionCommands(ctx context.Context, target Target, unset bool, pairs []optionValue) error {
	commands := make([][]string, 0, len(pairs))
	for _, pair := range pairs {
		if unset {
			commands = append(commands, []string{"set-option", "-p", "-u", "-t", target.PaneID, pair.key})
			continue
		}
		commands = append(commands, []string{"set-option", "-p", "-t", target.PaneID, pair.key, pair.value})
	}
	if len(commands) == 0 {
		return nil
	}
	_, err := c.run(ctx, nil, joinTmuxCommands(commands)...)
	if unset && errors.Is(classifyOptionError(err), ErrTmuxOptionUnavailable) {
		return nil
	}
	return err
}

func joinTmuxCommands(commands [][]string) []string {
	args := make([]string, 0, len(commands)*8)
	for i, command := range commands {
		if i > 0 {
			args = append(args, ";")
		}
		args = append(args, command...)
	}
	return args
}

type optionValue struct {
	key   string
	value string
}

func metadataOptionPairs(meta BridgeMetadata) []optionValue {
	opts := meta.ToOptions()
	return []optionValue{
		{key: OptionManaged, value: opts[OptionManaged]},
		{key: OptionMode, value: opts[OptionMode]},
		{key: OptionAgent, value: opts[OptionAgent]},
		{key: OptionLabel, value: opts[OptionLabel]},
		{key: OptionCreatedBy, value: opts[OptionCreatedBy]},
		{key: OptionLastActivity, value: opts[OptionLastActivity]},
	}
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

func parseUserOptions(output string) map[string]string {
	opts := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "@tmuxconn_") {
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

func paneListFormat() string {
	return strings.Join([]string{
		"#{pane_id}",
		"#{session_name}",
		"#{window_id}",
		"#{window_name}",
		"#{pane_title}",
		"#{pane_current_command}",
		"#{pane_current_path}",
		"#{pane_dead}",
		"#{pane_width}",
		"#{pane_height}",
	}, paneFieldSep)
}

func paneStateFormat() string {
	return strings.Join([]string{
		"#{pane_id}",
		"#{session_name}",
		"#{window_id}",
		"#{window_name}",
		"#{pane_title}",
		"#{pane_current_command}",
		"#{pane_current_path}",
		"#{pane_dead}",
		"#{pane_width}",
		"#{pane_height}",
		"#{@tmuxconn_managed}",
		"#{@tmuxconn_mode}",
		"#{@tmuxconn_agent}",
		"#{@tmuxconn_label}",
		"#{@tmuxconn_created_by}",
		"#{@tmuxconn_last_activity_unix}",
	}, paneFieldSep)
}

func parsePaneStateLine(socket string, line string) (PaneInfo, BridgeMetadata, error) {
	fields := splitPaneFields(line, 16)
	if len(fields) != 16 {
		return PaneInfo{}, BridgeMetadata{}, fmt.Errorf("unexpected list-panes row: %q", line)
	}
	info, err := buildPaneInfo(socket, fields[:10])
	if err != nil {
		return PaneInfo{}, BridgeMetadata{}, err
	}
	meta := MetadataFromOptions(map[string]string{
		OptionManaged:      fields[10],
		OptionMode:         fields[11],
		OptionAgent:        fields[12],
		OptionLabel:        fields[13],
		OptionCreatedBy:    fields[14],
		OptionLastActivity: fields[15],
	})
	return info, meta, nil
}

func parsePaneInfoLine(socket string, line string) (PaneInfo, error) {
	fields := splitPaneFields(line, 10)
	if len(fields) != 10 {
		return PaneInfo{}, fmt.Errorf("unexpected list-panes row: %q", line)
	}
	return buildPaneInfo(socket, fields)
}

func splitPaneFields(line string, expected int) []string {
	fields := strings.Split(line, paneFieldSep)
	if len(fields) == expected {
		return fields
	}
	if strings.Contains(line, paneFieldSepEscaped) {
		fields = strings.Split(line, paneFieldSepEscaped)
		if len(fields) == expected {
			return fields
		}
	}
	return fields
}

func buildPaneInfo(socket string, fields []string) (PaneInfo, error) {
	width, err := strconv.Atoi(fields[8])
	if err != nil {
		return PaneInfo{}, fmt.Errorf("parse width for %s: %w", fields[0], err)
	}
	height, err := strconv.Atoi(fields[9])
	if err != nil {
		return PaneInfo{}, fmt.Errorf("parse height for %s: %w", fields[0], err)
	}
	return PaneInfo{
		Target:      Target{Socket: socket, PaneID: fields[0]},
		SessionName: fields[1],
		WindowID:    fields[2],
		WindowName:  fields[3],
		PaneTitle:   fields[4],
		CurrentCmd:  fields[5],
		CurrentPath: fields[6],
		Dead:        fields[7] == "1",
		Width:       width,
		Height:      height,
	}, nil
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
