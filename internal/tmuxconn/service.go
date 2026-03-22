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

func (s *Service) Send(ctx context.Context, ref string, text string, sendEnter bool) error {
	return s.send(ctx, ref, text, sendEnter, false)
}

func (s *Service) SendManaged(ctx context.Context, ref string, text string, sendEnter bool) error {
	return s.send(ctx, ref, text, sendEnter, true)
}

func (s *Service) SendKeys(ctx context.Context, ref string, keys ...string) error {
	return s.sendKeys(ctx, ref, false, keys...)
}

func (s *Service) SendKeysManaged(ctx context.Context, ref string, keys ...string) error {
	return s.sendKeys(ctx, ref, true, keys...)
}

func (s *Service) send(ctx context.Context, ref string, text string, sendEnter bool, managed bool) error {
	pane, err := s.ResolvePane(ctx, ref)
	if err != nil {
		return err
	}
	if err := s.tmux.InjectInput(ctx, pane.Target, []byte(text)); err != nil {
		return TmuxError("send input to %s: %v", pane.Target.PaneKey(), err)
	}
	if sendEnter {
		if err := s.tmux.SendKeys(ctx, pane.Target, "Enter"); err != nil {
			return TmuxError("send enter to %s: %v", pane.Target.PaneKey(), err)
		}
	}
	if err := s.touchPaneMetadata(ctx, pane.Target, managed); err != nil {
		return TmuxError("update metadata for %s: %v", pane.Target.PaneKey(), err)
	}
	return nil
}

func (s *Service) Enter(ctx context.Context, ref string) error {
	return s.SendKeys(ctx, ref, "Enter")
}

func (s *Service) CtrlC(ctx context.Context, ref string) error {
	return s.SendKeys(ctx, ref, "C-c")
}

func (s *Service) EnterManaged(ctx context.Context, ref string) error {
	return s.SendKeysManaged(ctx, ref, "Enter")
}

func (s *Service) CtrlCManaged(ctx context.Context, ref string) error {
	return s.SendKeysManaged(ctx, ref, "C-c")
}

type PaneStream struct {
	Pane         tmux.PaneInfo
	Initial      string
	Subscription *tmux.Subscription
}

func (s *Service) sendKeys(ctx context.Context, ref string, managed bool, keys ...string) error {
	if len(keys) == 0 {
		return UsageError("send keys requires at least one key")
	}
	pane, err := s.ResolvePane(ctx, ref)
	if err != nil {
		return err
	}
	if err := s.tmux.SendKeys(ctx, pane.Target, keys...); err != nil {
		return TmuxError("send keys %q to %s: %v", strings.Join(keys, " "), pane.Target.PaneKey(), err)
	}
	if err := s.touchPaneMetadata(ctx, pane.Target, managed); err != nil {
		return TmuxError("update metadata for %s: %v", pane.Target.PaneKey(), err)
	}
	return nil
}

func (s *Service) OpenStream(ctx context.Context, ref string, lines int) (PaneStream, error) {
	pane, err := s.ResolvePane(ctx, ref)
	if err != nil {
		return PaneStream{}, err
	}
	initial, stream, err := s.tmux.OpenPaneStream(ctx, pane, lines)
	if err != nil {
		return PaneStream{}, TmuxError("open stream for %s: %v", pane.Target.PaneKey(), err)
	}
	return PaneStream{Pane: pane, Initial: initial, Subscription: stream}, nil
}

func (s *Service) touchPaneMetadata(ctx context.Context, target tmux.Target, managed bool) error {
	if managed {
		return s.tmux.TouchMetadataManaged(ctx, target)
	}
	return s.tmux.TouchMetadata(ctx, target)
}
