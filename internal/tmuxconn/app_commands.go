package tmuxconn

import (
	"context"
	"fmt"
)

func (a *App) runSnapshot(ctx context.Context, args []string) error {
	command := a.newPaneCommandFlags("snapshot")
	lines := command.fs.Int("lines", 120, "number of recent lines to capture")
	pane, jsonOut, err := command.parse(args)
	if err != nil {
		return err
	}
	body, err := a.service.Snapshot(ctx, pane, *lines)
	if err != nil {
		return err
	}
	return a.writeOutput(jsonOut, map[string]any{
		"pane":     pane,
		"lines":    *lines,
		"snapshot": body,
	}, func() error {
		_, err := fmt.Fprint(a.stdout, body)
		return err
	})
}

func (a *App) runSend(ctx context.Context, args []string) error {
	command := a.newPaneCommandFlags("send")
	text := command.fs.String("text", "", "text to inject")
	enter := command.fs.Bool("enter", false, "send Enter after text injection")
	pane, jsonOut, err := command.parse(args)
	if err != nil {
		return err
	}
	if *text == "" {
		return UsageError("send requires --text")
	}
	if err := a.service.Send(ctx, pane, *text, *enter); err != nil {
		return err
	}
	return a.writeOutput(jsonOut, map[string]any{
		"pane":  pane,
		"sent":  true,
		"enter": *enter,
	}, func() error {
		_, err := fmt.Fprintf(a.stdout, "sent input to %s\n", pane)
		return err
	})
}

func (a *App) runEnter(ctx context.Context, args []string) error {
	command := a.newPaneCommandFlags("enter")
	pane, jsonOut, err := command.parse(args)
	if err != nil {
		return err
	}
	if err := a.service.Enter(ctx, pane); err != nil {
		return err
	}
	return a.writeOutput(jsonOut, map[string]any{
		"pane": pane,
		"sent": true,
		"key":  "Enter",
	}, func() error {
		_, err := fmt.Fprintf(a.stdout, "sent Enter to %s\n", pane)
		return err
	})
}

func (a *App) runCtrlC(ctx context.Context, args []string) error {
	command := a.newPaneCommandFlags("ctrl-c")
	pane, jsonOut, err := command.parse(args)
	if err != nil {
		return err
	}
	if err := a.service.CtrlC(ctx, pane); err != nil {
		return err
	}
	return a.writeOutput(jsonOut, map[string]any{
		"pane": pane,
		"sent": true,
		"key":  "C-c",
	}, func() error {
		_, err := fmt.Fprintf(a.stdout, "sent Ctrl-C to %s\n", pane)
		return err
	})
}
