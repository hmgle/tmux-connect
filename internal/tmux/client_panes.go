package tmux

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

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
