package daemon

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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

func runDoctorWithConfig(ctx context.Context, stdout io.Writer, stderr io.Writer, service paneService, fileCfg config.Daemon, args []string) error {
	cfg, err := parseConfigWithFile(args, stderr, false, fileCfg)
	if err != nil {
		return err
	}

	fmt.Fprintln(stdout, "tmux-connect daemon doctor")

	switch cfg.Platform {
	case "telegram":
		if strings.TrimSpace(cfg.TelegramToken) == "" {
			return tmuxconn.UsageError("telegram token is required; pass --telegram-token, TMUXCONN_TELEGRAM_TOKEN, or [daemon.telegram].token in config")
		}
		fmt.Fprintln(stdout, "telegram token: ok")
	case "slack":
		if strings.TrimSpace(cfg.SlackBotToken) == "" {
			return tmuxconn.UsageError("slack bot token is required; pass --slack-bot-token, TMUXCONN_SLACK_BOT_TOKEN, or [daemon.slack].bot_token in config")
		}
		if strings.TrimSpace(cfg.SlackAppToken) == "" {
			return tmuxconn.UsageError("slack app token is required; pass --slack-app-token, TMUXCONN_SLACK_APP_TOKEN, or [daemon.slack].app_token in config")
		}
		fmt.Fprintln(stdout, "slack tokens: ok")
		fmt.Fprintln(stdout, "slack bot scopes: ensure app_mentions:read, chat:write, files:write, im:history, im:read")
		fmt.Fprintln(stdout, "slack snapshot uploads: uses the current Web API upload flow; reinstall the app after scope changes")
	case "discord":
		if strings.TrimSpace(cfg.DiscordToken) == "" {
			return tmuxconn.UsageError("discord token is required; pass --discord-token, TMUXCONN_DISCORD_TOKEN, or [daemon.discord].token in config")
		}
		fmt.Fprintln(stdout, "discord token: ok")
		fmt.Fprintln(stdout, "discord gateway intents: enable Message Content intent for prefix commands and DMs")
	case "whatsapp":
		if strings.TrimSpace(cfg.WhatsAppSessionDB) == "" {
			return tmuxconn.UsageError("whatsapp session db is required; pass --whatsapp-session-db, TMUXCONN_WHATSAPP_SESSION_DB, or [daemon.whatsapp].session_db in config")
		}
		if err := os.MkdirAll(filepath.Dir(cfg.WhatsAppSessionDB), 0o755); err != nil {
			return tmuxconn.UsageError("prepare whatsapp session db dir: %v", err)
		}
		fmt.Fprintf(stdout, "whatsapp session db: ok (%s)\n", cfg.WhatsAppSessionDB)
		fmt.Fprintln(stdout, "whatsapp login: first run will print a pairing QR code if no device session exists")
		if cfg.WhatsAppAllowSelfChat {
			fmt.Fprintln(stdout, "whatsapp self-chat: enabled (plain text is disabled; use explicit slash commands in self-chat)")
		}
	default:
		return tmuxconn.UsageError("unsupported platform %q", cfg.Platform)
	}

	store, err := OpenStore(ctx, cfg.DBPath)
	if err != nil {
		return tmuxconn.UsageError("open sqlite store: %v", err)
	}
	fmt.Fprintf(stdout, "sqlite store: ok (%s)\n", store.Path())

	registry := NewPaneRegistry(service)
	if err := registry.Refresh(ctx); err != nil {
		return tmuxconn.TmuxError("list panes: %v", err)
	}
	fmt.Fprintf(stdout, "tmux panes: ok (%d managed)\n", registry.ManagedCount())
	return nil
}

func runStatusWithConfig(ctx context.Context, stdout io.Writer, stderr io.Writer, service paneService, fileCfg config.Daemon, args []string) error {
	cfg, err := parseConfigWithFile(args, stderr, false, fileCfg)
	if err != nil {
		return err
	}
	store, err := OpenStore(ctx, cfg.DBPath)
	if err != nil {
		return tmuxconn.UsageError("open sqlite store: %v", err)
	}
	stats, err := store.Stats(ctx)
	if err != nil {
		return tmuxconn.UsageError("read sqlite stats: %v", err)
	}
	registry := NewPaneRegistry(service)
	refreshErr := registry.Refresh(ctx)

	fmt.Fprintln(stdout, "tmux-connect daemon status")
	fmt.Fprintf(stdout, "db: %s\n", cfg.DBPath)
	fmt.Fprintf(stdout, "registered chats: %d\n", stats.Chats)
	fmt.Fprintf(stdout, "bindings: %d\n", stats.Bindings)
	fmt.Fprintf(stdout, "message log rows: %d\n", stats.Messages)
	if refreshErr != nil {
		fmt.Fprintf(stdout, "tmux: error: %v\n", refreshErr)
		return nil
	}
	fmt.Fprintf(stdout, "managed panes: %d\n", registry.ManagedCount())
	return nil
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `tmux-connect daemon manages remote relay access for tmux panes.

Usage:
  tmux-connect daemon <command> [flags]

Commands:
  run      Start the relay daemon
  doctor   Validate token, sqlite store, and tmux access
  status   Show sqlite counts and current managed pane count

Common flags:
  --platform telegram|slack|discord|whatsapp
  --telegram-token TOKEN
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
`)
}
