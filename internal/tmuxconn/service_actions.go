package tmuxconn

import (
	"context"
	"strings"

	"github.com/hmgle/tmux-connect/internal/tmux"
)

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
