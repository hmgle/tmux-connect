package tagb

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/portgle/tmux-connect/internal/tmux"
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
	panes, err := s.tmux.ListPanes(ctx)
	if err != nil {
		return nil, TmuxError("list panes: %v", err)
	}

	records := make([]PaneRecord, 0, len(panes))
	for _, pane := range panes {
		meta, err := s.tmux.GetMetadata(ctx, pane.Target)
		if err != nil {
			return nil, TmuxError("read metadata for %s: %v", pane.Target.PaneKey(), err)
		}
		records = append(records, PaneRecord{Info: pane, Metadata: meta})
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

func (s *Service) ResolvePane(ctx context.Context, ref string) (tmux.PaneInfo, error) {
	target, err := tmux.ParseTarget(ref, s.tmux.SocketName())
	if err != nil {
		return tmux.PaneInfo{}, UsageError("%v", err)
	}
	panes, err := s.tmux.ListPanes(ctx)
	if err != nil {
		return tmux.PaneInfo{}, TmuxError("list panes: %v", err)
	}
	for _, pane := range panes {
		if pane.Target.Matches(target) {
			return pane, nil
		}
	}
	return tmux.PaneInfo{}, NotFoundError("pane not found: %s", ref)
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
	pane, err := s.ResolvePane(ctx, ref)
	if err != nil {
		return PaneRecord{}, err
	}
	meta, err := s.tmux.GetMetadata(ctx, pane.Target)
	if err != nil {
		return PaneRecord{}, TmuxError("read metadata for %s: %v", pane.Target.PaneKey(), err)
	}
	return PaneRecord{Info: pane, Metadata: meta}, nil
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

func (s *Service) Send(ctx context.Context, ref string, text string, sendEnter bool) error {
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
	if err := s.tmux.TouchMetadata(ctx, pane.Target); err != nil {
		return TmuxError("update metadata for %s: %v", pane.Target.PaneKey(), err)
	}
	return nil
}

func (s *Service) Enter(ctx context.Context, ref string) error {
	_, err := s.sendControlKey(ctx, ref, "Enter")
	return err
}

func (s *Service) CtrlC(ctx context.Context, ref string) error {
	_, err := s.sendControlKey(ctx, ref, "C-c")
	return err
}

type PaneStream struct {
	Pane         tmux.PaneInfo
	Initial      string
	Subscription *tmux.Subscription
}

func (s *Service) sendControlKey(ctx context.Context, ref string, key string) (tmux.PaneInfo, error) {
	pane, err := s.ResolvePane(ctx, ref)
	if err != nil {
		return tmux.PaneInfo{}, err
	}
	if err := s.tmux.SendKeys(ctx, pane.Target, key); err != nil {
		return tmux.PaneInfo{}, TmuxError("send %s to %s: %v", key, pane.Target.PaneKey(), err)
	}
	if err := s.tmux.TouchMetadata(ctx, pane.Target); err != nil {
		return tmux.PaneInfo{}, TmuxError("update metadata for %s: %v", pane.Target.PaneKey(), err)
	}
	return pane, nil
}

func (s *Service) OpenStream(ctx context.Context, ref string, lines int) (PaneStream, error) {
	pane, err := s.ResolvePane(ctx, ref)
	if err != nil {
		return PaneStream{}, err
	}
	initial, err := s.tmux.CapturePane(ctx, pane.Target, lines)
	if err != nil {
		return PaneStream{}, TmuxError("capture pane %s: %v", pane.Target.PaneKey(), err)
	}
	stream, err := s.tmux.SubscribePane(ctx, pane)
	if err != nil {
		return PaneStream{}, TmuxError("subscribe to %s: %v", pane.Target.PaneKey(), err)
	}
	return PaneStream{Pane: pane, Initial: initial, Subscription: stream}, nil
}
