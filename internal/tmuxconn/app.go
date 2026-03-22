package tmuxconn

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/hmgle/tmux-connect/internal/buildinfo"
	"github.com/hmgle/tmux-connect/internal/config"
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
	case "version":
		return a.runVersion(args[1:])
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

func (a *App) printUsage() {
	defaultConfigPath := "$XDG_CONFIG_HOME/" + config.DefaultDirName + `/config.toml`
	fmt.Fprintf(a.stderr, `tmux-connect manages tmux panes for local relay workflows.

Usage:
  tmux-connect [--config PATH] [--socket NAME] [--json] <command> [flags]

Global flags:
  --config PATH  load TOML config (default: %s)
  --socket NAME  tmux socket name
  --json         pass --json to the selected command
  note: global flags must appear before the command

Commands:
  version  [--json]
  list [--json]
  attach   --pane %%5 [--agent unknown] [--label api] [--json]
  detach   --pane %%5 [--json]
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

func (a *App) runVersion(args []string) error {
	command := a.newCommandFlags("version")
	jsonOut, err := command.parse(args)
	if err != nil {
		return err
	}

	info := buildinfo.Current()
	return a.writeOutput(jsonOut, info, func() error {
		_, err := fmt.Fprintf(
			a.stdout,
			"tmux-connect %s\ncommit: %s\nbuilt: %s\n",
			info.Version,
			info.Commit,
			info.Date,
		)
		return err
	})
}
