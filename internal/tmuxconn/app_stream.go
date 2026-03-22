package tmuxconn

import (
	"context"
	"fmt"
)

func (a *App) runStream(ctx context.Context, args []string) error {
	command := a.newPaneCommandFlags("stream")
	lines := command.fs.Int("lines", 120, "number of recent lines to print before following output")
	pane, jsonOut, err := command.parse(args)
	if err != nil {
		return err
	}
	stream, err := a.service.OpenStream(ctx, pane, *lines)
	if err != nil {
		return err
	}
	defer stream.Subscription.Close()

	if err := a.writeOutput(jsonOut, map[string]any{
		"event":   "initial",
		"pane":    stream.Pane.Target.PaneKey(),
		"lines":   *lines,
		"content": stream.Initial,
	}, func() error {
		if stream.Initial == "" {
			return nil
		}
		_, err := fmt.Fprint(a.stdout, stream.Initial)
		return err
	}); err != nil {
		return err
	}

	chunks := stream.Subscription.Chunks()
	errs := stream.Subscription.Errs()
	for {
		if chunks == nil && errs == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err == nil {
				continue
			}
			return TmuxError("stream %s: %v", stream.Pane.Target.PaneKey(), err)
		case chunk, ok := <-chunks:
			if !ok {
				chunks = nil
				continue
			}
			if err := a.writeOutput(jsonOut, map[string]any{
				"event":   "output",
				"pane":    stream.Pane.Target.PaneKey(),
				"content": chunk.Text,
				"at":      chunk.ReceivedAt,
			}, func() error {
				_, err := fmt.Fprint(a.stdout, chunk.Text)
				return err
			}); err != nil {
				return err
			}
		}
	}
}
