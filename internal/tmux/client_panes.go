package tmux

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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
	output, err := c.listPaneOutput(ctx, extraArgs, paneListFormat())
	if err != nil {
		return nil, err
	}

	lines := paneOutputLines(output)
	panes := make([]PaneInfo, 0, len(lines))
	for _, line := range lines {
		pane, parseErr := parsePaneInfoLine(c.SocketName(), line)
		if parseErr != nil {
			return nil, parseErr
		}
		panes = append(panes, pane)
	}
	return panes, nil
}

func (c *Client) listPaneStates(ctx context.Context, extraArgs []string) ([]PaneState, error) {
	output, err := c.listPaneOutput(ctx, extraArgs, paneStateFormat())
	if err != nil {
		return nil, err
	}

	lines := paneOutputLines(output)
	states := make([]PaneState, 0, len(lines))
	for _, line := range lines {
		pane, meta, parseErr := parsePaneStateLine(c.SocketName(), line)
		if parseErr != nil {
			return nil, parseErr
		}
		states = append(states, PaneState{Info: pane, Metadata: meta})
	}
	return states, nil
}

func (c *Client) listPaneOutput(ctx context.Context, extraArgs []string, format string) (string, error) {
	args := []string{"list-panes"}
	args = append(args, extraArgs...)
	args = append(args, "-F", format)
	return c.run(ctx, nil, args...)
}
