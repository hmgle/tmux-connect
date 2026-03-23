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
	case "feishu":
		if strings.TrimSpace(cfg.FeishuAppID) == "" {
			return tmuxconn.UsageError("feishu app id is required; pass --feishu-app-id, TMUXCONN_FEISHU_APP_ID, or [daemon.feishu].app_id in config")
		}
		if strings.TrimSpace(cfg.FeishuAppSecret) == "" {
			return tmuxconn.UsageError("feishu app secret is required; pass --feishu-app-secret, TMUXCONN_FEISHU_APP_SECRET, or [daemon.feishu].app_secret in config")
		}
		fmt.Fprintln(stdout, "feishu app credentials: ok")
		fmt.Fprintln(stdout, "feishu bot capability: enable bot ability and subscribe to im.message.receive_v1")
		fmt.Fprintln(stdout, "feishu group behavior: the bot only handles @mentions in group chats")
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
