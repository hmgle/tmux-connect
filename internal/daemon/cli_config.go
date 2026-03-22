package daemon

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hmgle/tmux-connect/internal/config"
	"github.com/hmgle/tmux-connect/internal/termrender"
	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

func parseConfig(args []string, stderr io.Writer, requireRun bool) (Config, error) {
	return parseConfigWithFile(args, stderr, requireRun, config.Daemon{})
}

func parseConfigWithFile(args []string, stderr io.Writer, requireRun bool, fileCfg config.Daemon) (Config, error) {
	defaults, err := resolveConfigDefaults(fileCfg)
	if err != nil {
		return Config{}, err
	}

	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg := Config{}
	values := bindConfigFlags(fs, &cfg, defaults)

	if err := fs.Parse(args); err != nil {
		return Config{}, tmuxconn.UsageError("%v", err)
	}
	cfg.Platform = strings.TrimSpace(strings.ToLower(cfg.Platform))
	if cfg.Platform == "" {
		cfg.Platform = "telegram"
	}
	cfg.PlainTextMode = plainTextMode(values.plainTextMode)
	cfg.PlainTextEcho = plainTextEchoMode(values.plainTextEcho)
	switch {
	case flagWasSet(fs, "allow-chat"):
		cfg.AllowChats = append([]string(nil), values.allowChats.values...)
	case fileCfg.AllowChats != nil:
		cfg.AllowChats = append([]string(nil), (*fileCfg.AllowChats)...)
	}
	if cfg.PollTimeout <= 0 {
		return Config{}, tmuxconn.UsageError("--poll-timeout must be > 0")
	}
	if cfg.SnapshotLines <= 0 {
		return Config{}, tmuxconn.UsageError("--snapshot-lines must be > 0")
	}
	if cfg.PlainTextMode, err = parsePlainTextMode(string(cfg.PlainTextMode)); err != nil {
		return Config{}, tmuxconn.UsageError("%v", err)
	}
	if cfg.PlainTextEcho, err = parsePlainTextEchoMode(string(cfg.PlainTextEcho)); err != nil {
		return Config{}, tmuxconn.UsageError("%v", err)
	}
	if cfg.PlainTextEchoLines <= 0 {
		return Config{}, tmuxconn.UsageError("--plain-text-echo-lines must be > 0")
	}
	if cfg.PlainTextEchoDelay <= 0 {
		return Config{}, tmuxconn.UsageError("--plain-text-echo-delay must be > 0")
	}
	if cfg.PlainTextEchoTimeout <= 0 {
		return Config{}, tmuxconn.UsageError("--plain-text-echo-timeout must be > 0")
	}
	if err := termrender.ValidateOptions(snapshotRenderOptions(cfg)); err != nil {
		return Config{}, tmuxconn.UsageError("%v", err)
	}
	if cfg.FollowLines <= 0 {
		return Config{}, tmuxconn.UsageError("--follow-lines must be > 0")
	}
	if cfg.FollowMinGap <= 0 {
		return Config{}, tmuxconn.UsageError("--follow-min-interval must be > 0")
	}
	if shouldUseWhatsAppFollowMinGap(fs, fileCfg, cfg.Platform) {
		cfg.FollowMinGap = 2 * time.Second
	}
	if cfg.Platform == "slack" {
		if strings.TrimSpace(cfg.SlackCommandPrefix) == "" || strings.ContainsAny(cfg.SlackCommandPrefix, " \t\n") {
			return Config{}, tmuxconn.UsageError("--slack-command-prefix must be non-empty and contain no whitespace")
		}
	} else {
		cfg.SlackCommandPrefix = ""
	}
	if cfg.Platform == "discord" {
		if strings.TrimSpace(cfg.DiscordCommandPrefix) == "" || strings.ContainsAny(cfg.DiscordCommandPrefix, " \t\n") {
			return Config{}, tmuxconn.UsageError("--discord-command-prefix must be non-empty and contain no whitespace")
		}
	} else {
		cfg.DiscordCommandPrefix = ""
	}
	if strings.TrimSpace(cfg.WhatsAppSessionDB) == "" {
		cfg.WhatsAppSessionDB = defaultWhatsAppSessionDBPath(cfg.DBPath)
	}
	if !flagWasSet(fs, "whatsapp-session-db") && fileCfg.WhatsApp.SessionDB == nil && strings.TrimSpace(os.Getenv("TMUXCONN_WHATSAPP_SESSION_DB")) == "" {
		cfg.WhatsAppSessionDB = defaultWhatsAppSessionDBPath(cfg.DBPath)
	}
	if strings.TrimSpace(cfg.WhatsAppDeviceName) == "" {
		cfg.WhatsAppDeviceName = "tmux-connect"
	}
	if requireRun {
		switch cfg.Platform {
		case "telegram":
			if strings.TrimSpace(cfg.TelegramToken) == "" {
				return Config{}, tmuxconn.UsageError("daemon run requires --telegram-token, TMUXCONN_TELEGRAM_TOKEN, or [daemon.telegram].token in config")
			}
		case "slack":
			if strings.TrimSpace(cfg.SlackBotToken) == "" || strings.TrimSpace(cfg.SlackAppToken) == "" {
				return Config{}, tmuxconn.UsageError("daemon run requires --slack-bot-token/--slack-app-token, TMUXCONN_SLACK_BOT_TOKEN/TMUXCONN_SLACK_APP_TOKEN, or [daemon.slack].bot_token/[daemon.slack].app_token in config")
			}
		case "discord":
			if strings.TrimSpace(cfg.DiscordToken) == "" {
				return Config{}, tmuxconn.UsageError("daemon run requires --discord-token, TMUXCONN_DISCORD_TOKEN, or [daemon.discord].token in config")
			}
		case "whatsapp":
			if strings.TrimSpace(cfg.WhatsAppSessionDB) == "" {
				return Config{}, tmuxconn.UsageError("daemon run requires --whatsapp-session-db, TMUXCONN_WHATSAPP_SESSION_DB, or [daemon.whatsapp].session_db in config")
			}
		default:
			return Config{}, tmuxconn.UsageError("unsupported --platform %q", cfg.Platform)
		}
	}
	if strings.TrimSpace(cfg.DBPath) == "" {
		return Config{}, tmuxconn.UsageError("--db is required")
	}
	return cfg, nil
}

func defaultWhatsAppSessionDBPath(dbPath string) string {
	dbPath = strings.TrimSpace(dbPath)
	if dbPath == "" {
		return filepath.Join(filepath.Dir(defaultDBPath()), "whatsapp-device.db")
	}
	return filepath.Join(filepath.Dir(dbPath), "whatsapp-device.db")
}

func envOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value != "" {
		return value
	}
	return fallback
}

func envFloat64(key string, fallback float64) (float64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number", key)
	}
	return parsed, nil
}

func envIntValue(key string) (int, bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0, false, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, true, fmt.Errorf("%s must be an integer", key)
	}
	return parsed, true, nil
}

func envDurationValue(key string, fallback time.Duration) (time.Duration, bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, false, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, true, fmt.Errorf("%s must be a duration", key)
	}
	return parsed, true, nil
}

func envBoolValue(key string) (bool, bool) {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return false, false
	}
	switch value {
	case "1", "true", "yes", "on":
		return true, true
	default:
		return false, true
	}
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

func stringValue(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return strings.TrimSpace(*value)
}

func intValue(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func float64Value(value *float64, fallback float64) float64 {
	if value == nil {
		return fallback
	}
	return *value
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func durationValue(fieldName string, value *string, fallback time.Duration) (time.Duration, error) {
	if value == nil {
		return fallback, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return 0, fmt.Errorf("%s must be a duration", fieldName)
	}
	parsed, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", fieldName, err)
	}
	return parsed, nil
}

func parsePlainTextMode(value string) (plainTextMode, error) {
	switch mode := plainTextMode(strings.TrimSpace(strings.ToLower(value))); mode {
	case plainTextModeType, plainTextModeExecute:
		return mode, nil
	default:
		return "", fmt.Errorf("--plain-text-mode must be type or execute")
	}
}

func parsePlainTextEchoMode(value string) (plainTextEchoMode, error) {
	switch mode := plainTextEchoMode(strings.TrimSpace(strings.ToLower(value))); mode {
	case plainTextEchoOff, plainTextEchoSnapshot:
		return mode, nil
	default:
		return "", fmt.Errorf("--plain-text-echo must be off or snapshot")
	}
}

func normalizePlainTextConfig(cfg PlainTextConfig) PlainTextConfig {
	if cfg.Mode == "" {
		cfg.Mode = plainTextModeType
	}
	if cfg.Echo == "" {
		cfg.Echo = plainTextEchoSnapshot
	}
	if cfg.EchoLines <= 0 {
		cfg.EchoLines = 12
	}
	if cfg.EchoDelay <= 0 {
		cfg.EchoDelay = 250 * time.Millisecond
	}
	if cfg.EchoTimeout <= 0 {
		cfg.EchoTimeout = 2 * time.Second
	}
	return cfg
}
