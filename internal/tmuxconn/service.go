package tmuxconn

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/hmgle/tmux-connect/internal/tmux"
)

type Service struct {
	tmux *tmux.Client
}

func NewService(client *tmux.Client) *Service {
	return &Service{tmux: client}
}

type PaneRecord struct {
	Info     tmux.PaneInfo       `json:"info"`
	Metadata tmux.BridgeMetadata `json:"metadata"`
}

func (s *Service) List(ctx context.Context) ([]PaneRecord, error) {
	states, err := s.tmux.ListPaneStates(ctx)
	if err != nil {
		return nil, TmuxError("list panes: %v", err)
	}

	records := make([]PaneRecord, 0, len(states))
	for _, state := range states {
		records = append(records, PaneRecord{Info: state.Info, Metadata: state.Metadata})
	}

	slices.SortFunc(records, func(a, b PaneRecord) int {
		if a.Info.SessionName != b.Info.SessionName {
			return strings.Compare(a.Info.SessionName, b.Info.SessionName)
		}
		if a.Info.WindowName != b.Info.WindowName {
			return strings.Compare(a.Info.WindowName, b.Info.WindowName)
		}
		return strings.Compare(a.Info.Target.PaneID, b.Info.Target.PaneID)
	})
	return records, nil
}

func (s *Service) Attach(ctx context.Context, ref string, agent string, label string) (PaneRecord, error) {
	pane, err := s.ResolvePane(ctx, ref)
	if err != nil {
		return PaneRecord{}, err
	}
	meta := tmux.BridgeMetadata{
		Managed:          true,
		Mode:             tmux.ModeRelay,
		Agent:            tmux.NormalizeAgent(agent),
		Label:            strings.TrimSpace(label),
		CreatedBy:        tmux.CreatedByManualAttach,
		LastActivityUnix: time.Now().Unix(),
	}
	if err := s.tmux.SetMetadata(ctx, pane.Target, meta); err != nil {
		return PaneRecord{}, TmuxError("attach pane %s: %v", pane.Target.PaneKey(), err)
	}
	return PaneRecord{Info: pane, Metadata: meta}, nil
}

func (s *Service) Detach(ctx context.Context, ref string) error {
	pane, err := s.ResolvePane(ctx, ref)
	if err != nil {
		return err
	}
	if err := s.tmux.ClearMetadata(ctx, pane.Target); err != nil {
		return TmuxError("detach pane %s: %v", pane.Target.PaneKey(), err)
	}
	return nil
}

func (s *Service) Inspect(ctx context.Context, ref string) (PaneRecord, error) {
	target, err := s.parseTarget(ref)
	if err != nil {
		return PaneRecord{}, err
	}
	state, err := s.tmux.GetPaneState(ctx, target)
	if err != nil {
		return PaneRecord{}, classifyPaneLookupError("inspect pane", ref, err)
	}
	return PaneRecord{Info: state.Info, Metadata: state.Metadata}, nil
}

func (s *Service) Snapshot(ctx context.Context, ref string, lines int) (string, error) {
	pane, err := s.ResolvePane(ctx, ref)
	if err != nil {
		return "", err
	}
	body, err := s.tmux.CapturePane(ctx, pane.Target, lines)
	if err != nil {
		return "", TmuxError("capture pane %s: %v", pane.Target.PaneKey(), err)
	}
	return body, nil
}

func (s *Service) SnapshotRich(ctx context.Context, ref string, lines int) (string, error) {
	pane, err := s.ResolvePane(ctx, ref)
	if err != nil {
		return "", err
	}
	body, err := s.tmux.CapturePaneRich(ctx, pane.Target, lines)
	if err != nil {
		return "", TmuxError("capture rich pane %s: %v", pane.Target.PaneKey(), err)
	}
	return body, nil
}
