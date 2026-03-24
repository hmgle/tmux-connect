package tmuxconn

import (
	"context"
	"errors"
	"strings"

	"github.com/hmgle/tmux-connect/internal/tmux"
)

func (s *Service) ResolvePane(ctx context.Context, ref string) (tmux.PaneInfo, error) {
	target, err := tmux.ParseTarget(ref, s.tmux.SocketName())
	if err != nil {
		return tmux.PaneInfo{}, UsageError("%v", err)
	}
	if !target.Matches(tmux.Target{Socket: s.tmux.SocketName(), PaneID: target.PaneID}) {
		return tmux.PaneInfo{}, NotFoundError("pane not found: %s", ref)
	}
	pane, err := s.tmux.GetPane(ctx, target)
	if err != nil {
		return tmux.PaneInfo{}, classifyPaneLookupError("resolve pane", ref, err)
	}
	return pane, nil
}

func (s *Service) ResolvePaneState(ctx context.Context, ref string) (tmux.PaneState, error) {
	target, err := tmux.ParseTarget(ref, s.tmux.SocketName())
	if err != nil {
		return tmux.PaneState{}, UsageError("%v", err)
	}
	if !target.Matches(tmux.Target{Socket: s.tmux.SocketName(), PaneID: target.PaneID}) {
		return tmux.PaneState{}, NotFoundError("pane not found: %s", ref)
	}
	state, err := s.tmux.GetPaneState(ctx, target)
	if err != nil {
		return tmux.PaneState{}, classifyPaneLookupError("resolve pane", ref, err)
	}
	return state, nil
}

func (s *Service) parseTarget(ref string) (tmux.Target, error) {
	target, err := tmux.ParseTarget(ref, s.tmux.SocketName())
	if err != nil {
		return tmux.Target{}, UsageError("%v", err)
	}
	if !target.Matches(tmux.Target{Socket: s.tmux.SocketName(), PaneID: target.PaneID}) {
		return tmux.Target{}, NotFoundError("pane not found: %s", ref)
	}
	return target, nil
}

func classifyPaneLookupError(action string, ref string, err error) error {
	if err == nil {
		return nil
	}
	if isPaneNotFound(err) {
		return NotFoundError("pane not found: %s", ref)
	}
	return TmuxError("%s %s: %v", strings.TrimSpace(action), strings.TrimSpace(ref), err)
}

func isPaneNotFound(err error) bool {
	if err == nil {
		return false
	}
	var cmdErr *tmux.TmuxCommandError
	if errors.As(err, &cmdErr) {
		msg := strings.TrimSpace(cmdErr.Result.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(cmdErr.Result.Stdout)
		}
		if containsPaneNotFound(msg) {
			return true
		}
	}
	return containsPaneNotFound(err.Error())
}

func containsPaneNotFound(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(message, "pane not found") || strings.Contains(message, "can't find pane")
}
