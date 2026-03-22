package tmuxconn

import (
	"context"
	"fmt"
	"text/tabwriter"
)

func (a *App) runList(ctx context.Context, args []string) error {
	command := a.newCommandFlags("list")
	jsonOut, err := command.parse(args)
	if err != nil {
		return err
	}
	records, err := a.service.List(ctx)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(a.stdout, records)
	}
	tw := tabwriter.NewWriter(a.stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "PANE\tSESSION\tWINDOW\tCMD\tMANAGED\tAGENT\tMODE\tLABEL")
	for _, record := range records {
		fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			record.Info.Target.PaneKey(),
			record.Info.SessionName,
			record.Info.WindowName,
			record.Info.CurrentCmd,
			yesNo(record.Metadata.Managed),
			record.Metadata.Agent,
			record.Metadata.Mode,
			record.Metadata.Label,
		)
	}
	return tw.Flush()
}

func (a *App) runAttach(ctx context.Context, args []string) error {
	command := a.newPaneCommandFlags("attach")
	agent := command.fs.String("agent", "unknown", "agent label")
	label := command.fs.String("label", "", "human label for the pane")
	pane, jsonOut, err := command.parse(args)
	if err != nil {
		return err
	}
	record, err := a.service.Attach(ctx, pane, *agent, *label)
	if err != nil {
		return err
	}
	return a.writeOutput(jsonOut, record, func() error {
		_, err := fmt.Fprintf(a.stdout, "attached %s (%s)\n", record.Info.Target.PaneKey(), record.Info.SessionName)
		return err
	})
}

func (a *App) runDetach(ctx context.Context, args []string) error {
	command := a.newPaneCommandFlags("detach")
	pane, jsonOut, err := command.parse(args)
	if err != nil {
		return err
	}
	if err := a.service.Detach(ctx, pane); err != nil {
		return err
	}
	return a.writeOutput(jsonOut, map[string]any{
		"pane":     pane,
		"detached": true,
	}, func() error {
		_, err := fmt.Fprintf(a.stdout, "detached %s\n", pane)
		return err
	})
}

func (a *App) runInspect(ctx context.Context, args []string) error {
	command := a.newPaneCommandFlags("inspect")
	pane, jsonOut, err := command.parse(args)
	if err != nil {
		return err
	}
	record, err := a.service.Inspect(ctx, pane)
	if err != nil {
		return err
	}
	return a.writeOutput(jsonOut, record, func() error {
		fmt.Fprintf(a.stdout, "pane: %s\n", record.Info.Target.PaneKey())
		fmt.Fprintf(a.stdout, "session: %s\n", record.Info.SessionName)
		fmt.Fprintf(a.stdout, "window: %s\n", record.Info.WindowName)
		fmt.Fprintf(a.stdout, "command: %s\n", record.Info.CurrentCmd)
		fmt.Fprintf(a.stdout, "managed: %s\n", yesNo(record.Metadata.Managed))
		fmt.Fprintf(a.stdout, "mode: %s\n", record.Metadata.Mode)
		fmt.Fprintf(a.stdout, "agent: %s\n", record.Metadata.Agent)
		fmt.Fprintf(a.stdout, "label: %s\n", record.Metadata.Label)
		fmt.Fprintf(a.stdout, "created_by: %s\n", record.Metadata.CreatedBy)
		fmt.Fprintf(a.stdout, "last_activity: %s\n", formatLastActivity(record.Metadata.LastActivityUnix))
		return nil
	})
}
