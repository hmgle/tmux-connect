package daemon

import (
	"context"
	"fmt"
	"io"

	"github.com/hmgle/tmux-connect/internal/config"
	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

func RunCLI(ctx context.Context, stdout io.Writer, stderr io.Writer, service paneService, args []string) error {
	return RunCLIWithConfig(ctx, stdout, stderr, service, config.Daemon{}, args)
}

func RunCLIWithConfig(ctx context.Context, stdout io.Writer, stderr io.Writer, service paneService, fileCfg config.Daemon, args []string) error {
	if len(args) == 0 {
		printUsage(stderr)
		return tmuxconn.UsageError("missing daemon command")
	}
	switch args[0] {
	case "run":
		return runDaemonWithConfig(ctx, stdout, stderr, service, fileCfg, args[1:])
	case "doctor":
		return runDoctorWithConfig(ctx, stdout, stderr, service, fileCfg, args[1:])
	case "status":
		return runStatusWithConfig(ctx, stdout, stderr, service, fileCfg, args[1:])
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return tmuxconn.UsageError("unknown daemon command: %s", args[0])
	}
}

func runDaemonWithConfig(ctx context.Context, stdout io.Writer, stderr io.Writer, service paneService, fileCfg config.Daemon, args []string) error {
	cfg, err := parseConfigWithFile(args, stderr, true, fileCfg)
	if err != nil {
		return err
	}
	runtime, err := NewRuntime(ctx, cfg, service, stderr)
	if err != nil {
		return err
	}
	defer runtime.Close()

	if _, err := fmt.Fprintf(stdout, "tmux-connect daemon running with db %s\n", runtime.store.Path()); err != nil {
		return err
	}
	return runtime.Run(ctx)
}

func printUsage(w io.Writer) {
	platformSummary := availablePlatformSummary()
	if platformSummary == "" {
		platformSummary = "(none)"
	}
	defaultPlatform := defaultPlatformName()
	if defaultPlatform == "" {
		defaultPlatform = "(none)"
	}

	fmt.Fprintf(w, `tmux-connect daemon manages remote relay access for tmux panes.

Usage:
  tmux-connect daemon <command> [flags]

Commands:
  run      Start the relay daemon
  doctor   Validate token, sqlite store, and tmux access
  status   Show sqlite counts and current managed pane count

Compiled platforms:
  %s

Default platform:
  %s

Common flags:
  --platform PLATFORM
  --telegram-token TOKEN
  --feishu-app-id APP_ID
  --feishu-app-secret APP_SECRET
  --feishu-bot-open-id OPEN_ID
  --feishu-bot-user-id USER_ID
  --feishu-bot-union-id UNION_ID
  --slack-bot-token TOKEN
  --slack-app-token TOKEN
  --discord-token TOKEN
  --discord-command-prefix PREFIX
  --whatsapp-session-db PATH
  --whatsapp-device-name NAME
  --whatsapp-auto-mark-read
  --whatsapp-allow-self-chat
  --db PATH
  --allow-chat 123456
  --poll-timeout 20s
  --snapshot-lines 120
  --plain-text-mode type
  --plain-text-echo snapshot
  --plain-text-echo-lines 12
  --plain-text-echo-delay 250ms
  --plain-text-echo-timeout 2s
  --telegram-snapshot-theme dark
  --telegram-snapshot-font-size 14
  --telegram-snapshot-font-file /path/to/font.ttf
  --follow-lines 80
  --follow-min-interval 700ms
  --follow-debug
`, platformSummary, defaultPlatform)
}
