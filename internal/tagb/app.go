package tagb

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	tagbconfig "github.com/hmgle/tmux-connect/internal/config"
	"github.com/hmgle/tmux-connect/internal/tmux"
)

type App struct {
	stdout  io.Writer
	stderr  io.Writer
	service *Service
}

func NewApp(stdout io.Writer, stderr io.Writer, service *Service) *App {
	return &App{stdout: stdout, stderr: stderr, service: service}
}

func (a *App) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		a.printUsage()
		return UsageError("missing command")
	}

	switch args[0] {
	case "help", "-h", "--help":
		a.printUsage()
		return nil
	case "list":
		return a.runList(ctx, args[1:])
	case "attach":
		return a.runAttach(ctx, args[1:])
	case "detach":
		return a.runDetach(ctx, args[1:])
	case "inspect":
		return a.runInspect(ctx, args[1:])
	case "snapshot":
		return a.runSnapshot(ctx, args[1:])
	case "send":
		return a.runSend(ctx, args[1:])
	case "enter":
		return a.runEnter(ctx, args[1:])
	case "ctrl-c":
		return a.runCtrlC(ctx, args[1:])
	case "stream":
		return a.runStream(ctx, args[1:])
	default:
		a.printUsage()
		return UsageError("unknown command: %s", args[0])
	}
}

func (a *App) runList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	jsonOut := fs.Bool("json", false, "print machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return UsageError("%v", err)
	}
	records, err := a.service.List(ctx)
	if err != nil {
		return err
	}
	if *jsonOut {
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
	fs := flag.NewFlagSet("attach", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	pane := fs.String("pane", "", "pane id or pane key (required)")
	agent := fs.String("agent", string(tmux.AgentUnknown), "agent label")
	label := fs.String("label", "", "human label for the pane")
	jsonOut := fs.Bool("json", false, "print machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return UsageError("%v", err)
	}
	if strings.TrimSpace(*pane) == "" {
		return UsageError("attach requires --pane")
	}
	record, err := a.service.Attach(ctx, *pane, *agent, *label)
	if err != nil {
		return err
	}
	if *jsonOut {
		return writeJSON(a.stdout, record)
	}
	_, err = fmt.Fprintf(a.stdout, "attached %s (%s)\n", record.Info.Target.PaneKey(), record.Info.SessionName)
	return err
}

func (a *App) runDetach(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("detach", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	pane := fs.String("pane", "", "pane id or pane key (required)")
	if err := fs.Parse(args); err != nil {
		return UsageError("%v", err)
	}
	if strings.TrimSpace(*pane) == "" {
		return UsageError("detach requires --pane")
	}
	if err := a.service.Detach(ctx, *pane); err != nil {
		return err
	}
	_, err := fmt.Fprintf(a.stdout, "detached %s\n", *pane)
	return err
}

func (a *App) runInspect(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	pane := fs.String("pane", "", "pane id or pane key (required)")
	jsonOut := fs.Bool("json", false, "print machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return UsageError("%v", err)
	}
	if strings.TrimSpace(*pane) == "" {
		return UsageError("inspect requires --pane")
	}
	record, err := a.service.Inspect(ctx, *pane)
	if err != nil {
		return err
	}
	if *jsonOut {
		return writeJSON(a.stdout, record)
	}
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
}

func (a *App) runSnapshot(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	pane := fs.String("pane", "", "pane id or pane key (required)")
	lines := fs.Int("lines", 120, "number of recent lines to capture")
	jsonOut := fs.Bool("json", false, "print machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return UsageError("%v", err)
	}
	if strings.TrimSpace(*pane) == "" {
		return UsageError("snapshot requires --pane")
	}
	body, err := a.service.Snapshot(ctx, *pane, *lines)
	if err != nil {
		return err
	}
	if *jsonOut {
		return writeJSON(a.stdout, map[string]any{
			"pane":     *pane,
			"lines":    *lines,
			"snapshot": body,
		})
	}
	_, err = fmt.Fprint(a.stdout, body)
	return err
}

func (a *App) runSend(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	pane := fs.String("pane", "", "pane id or pane key (required)")
	text := fs.String("text", "", "text to inject")
	enter := fs.Bool("enter", false, "send Enter after text injection")
	jsonOut := fs.Bool("json", false, "print machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return UsageError("%v", err)
	}
	if strings.TrimSpace(*pane) == "" {
		return UsageError("send requires --pane")
	}
	if *text == "" {
		return UsageError("send requires --text")
	}
	if err := a.service.Send(ctx, *pane, *text, *enter); err != nil {
		return err
	}
	if *jsonOut {
		return writeJSON(a.stdout, map[string]any{
			"pane":  *pane,
			"sent":  true,
			"enter": *enter,
		})
	}
	_, err := fmt.Fprintf(a.stdout, "sent input to %s\n", *pane)
	return err
}

func (a *App) runEnter(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("enter", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	pane := fs.String("pane", "", "pane id or pane key (required)")
	jsonOut := fs.Bool("json", false, "print machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return UsageError("%v", err)
	}
	if strings.TrimSpace(*pane) == "" {
		return UsageError("enter requires --pane")
	}
	if err := a.service.Enter(ctx, *pane); err != nil {
		return err
	}
	if *jsonOut {
		return writeJSON(a.stdout, map[string]any{
			"pane": *pane,
			"sent": true,
			"key":  "Enter",
		})
	}
	_, err := fmt.Fprintf(a.stdout, "sent Enter to %s\n", *pane)
	return err
}

func (a *App) runCtrlC(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("ctrl-c", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	pane := fs.String("pane", "", "pane id or pane key (required)")
	jsonOut := fs.Bool("json", false, "print machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return UsageError("%v", err)
	}
	if strings.TrimSpace(*pane) == "" {
		return UsageError("ctrl-c requires --pane")
	}
	if err := a.service.CtrlC(ctx, *pane); err != nil {
		return err
	}
	if *jsonOut {
		return writeJSON(a.stdout, map[string]any{
			"pane": *pane,
			"sent": true,
			"key":  "C-c",
		})
	}
	_, err := fmt.Fprintf(a.stdout, "sent Ctrl-C to %s\n", *pane)
	return err
}

func (a *App) runStream(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("stream", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	pane := fs.String("pane", "", "pane id or pane key (required)")
	lines := fs.Int("lines", 120, "number of recent lines to print before following output")
	jsonOut := fs.Bool("json", false, "print machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return UsageError("%v", err)
	}
	if strings.TrimSpace(*pane) == "" {
		return UsageError("stream requires --pane")
	}
	stream, err := a.service.OpenStream(ctx, *pane, *lines)
	if err != nil {
		return err
	}
	defer stream.Subscription.Close()

	if *jsonOut {
		if err := writeJSON(a.stdout, map[string]any{
			"event":   "initial",
			"pane":    stream.Pane.Target.PaneKey(),
			"lines":   *lines,
			"content": stream.Initial,
		}); err != nil {
			return err
		}
	} else if stream.Initial != "" {
		if _, err := fmt.Fprint(a.stdout, stream.Initial); err != nil {
			return err
		}
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
			if *jsonOut {
				if err := writeJSON(a.stdout, map[string]any{
					"event":   "output",
					"pane":    stream.Pane.Target.PaneKey(),
					"content": chunk.Text,
					"at":      chunk.ReceivedAt,
				}); err != nil {
					return err
				}
				continue
			}
			if _, err := fmt.Fprint(a.stdout, chunk.Text); err != nil {
				return err
			}
		}
	}
}

func (a *App) printUsage() {
	defaultConfigPath := "$XDG_CONFIG_HOME/" + tagbconfig.DefaultDirName + `/config.toml`
	fmt.Fprintf(a.stderr, `tagb manages tmux panes for local relay workflows.

Usage:
  tagb [--config PATH] [--socket NAME] [--json] <command> [flags]

Global flags:
  --config PATH  load TOML config (default: %s)
  --socket NAME  tmux socket name
  --json         pass --json to the selected command

Commands:
  list [--json]
  attach   --pane %%5 [--agent unknown] [--label api] [--json]
  detach   --pane %%5
  inspect  --pane %%5 [--json]
  snapshot --pane %%5 [--lines 120] [--json]
  send     --pane %%5 --text "hello" [--enter] [--json]
  enter    --pane %%5 [--json]
  ctrl-c   --pane %%5 [--json]
  stream   --pane %%5 [--lines 120] [--json]
  serve    [--listen 127.0.0.1:8080]
  daemon   <run|doctor|status> [flags]
`, defaultConfigPath)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func formatLastActivity(unix int64) string {
	if unix <= 0 {
		return "-"
	}
	return time.Unix(unix, 0).Format(time.RFC3339)
}
