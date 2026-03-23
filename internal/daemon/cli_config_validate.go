package daemon

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hmgle/tmux-connect/internal/config"
	"github.com/hmgle/tmux-connect/internal/termrender"
	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

func applyConfigValues(fs *flag.FlagSet, cfg *Config, values *daemonConfigFlagValues, fileCfg config.Daemon) error {
	cfg.Platform = strings.TrimSpace(strings.ToLower(cfg.Platform))
	if cfg.Platform == "" {
		cfg.Platform = defaultPlatformName()
	}
	cfg.PlainTextMode = plainTextMode(values.plainTextMode)
	cfg.PlainTextEcho = plainTextEchoMode(values.plainTextEcho)
	switch {
	case flagWasSet(fs, "allow-chat"):
		cfg.AllowChats = append([]string(nil), values.allowChats.values...)
	case fileCfg.AllowChats != nil:
		cfg.AllowChats = append([]string(nil), (*fileCfg.AllowChats)...)
	}
	return nil
}

func validateConfig(cfg *Config, fs *flag.FlagSet, fileCfg config.Daemon, requireRun bool) error {
	if !isPlatformAvailable(cfg.Platform) {
		return tmuxconn.UsageError("%v", unsupportedPlatformError(cfg.Platform))
	}
	if err := validateConfigValues(cfg); err != nil {
		return err
	}
	applyPlatformSpecificConfigDefaults(cfg, fs, fileCfg)
	if err := validatePlatformPrefixes(cfg); err != nil {
		return err
	}
	if requireRun {
		if err := validateRunRequirements(*cfg); err != nil {
			return err
		}
	}
	if strings.TrimSpace(cfg.DBPath) == "" {
		return tmuxconn.UsageError("--db is required")
	}
	return nil
}

func validateConfigValues(cfg *Config) error {
	var err error
	if cfg.PollTimeout <= 0 {
		return tmuxconn.UsageError("--poll-timeout must be > 0")
	}
	if cfg.SnapshotLines <= 0 {
		return tmuxconn.UsageError("--snapshot-lines must be > 0")
	}
	if cfg.PlainTextMode, err = parsePlainTextMode(string(cfg.PlainTextMode)); err != nil {
		return tmuxconn.UsageError("%v", err)
	}
	if cfg.PlainTextEcho, err = parsePlainTextEchoMode(string(cfg.PlainTextEcho)); err != nil {
		return tmuxconn.UsageError("%v", err)
	}
	if cfg.PlainTextEchoLines <= 0 {
		return tmuxconn.UsageError("--plain-text-echo-lines must be > 0")
	}
	if cfg.PlainTextEchoDelay <= 0 {
		return tmuxconn.UsageError("--plain-text-echo-delay must be > 0")
	}
	if cfg.PlainTextEchoTimeout <= 0 {
		return tmuxconn.UsageError("--plain-text-echo-timeout must be > 0")
	}
	if err := termrender.ValidateOptions(snapshotRenderOptions(*cfg)); err != nil {
		return tmuxconn.UsageError("%v", err)
	}
	if cfg.FollowLines <= 0 {
		return tmuxconn.UsageError("--follow-lines must be > 0")
	}
	if cfg.FollowMinGap <= 0 {
		return tmuxconn.UsageError("--follow-min-interval must be > 0")
	}
	return nil
}

func applyPlatformSpecificConfigDefaults(cfg *Config, fs *flag.FlagSet, fileCfg config.Daemon) {
	if shouldUseWhatsAppFollowMinGap(fs, fileCfg, cfg.Platform) {
		cfg.FollowMinGap = 2 * time.Second
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
	if cfg.Platform != "slack" {
		cfg.SlackCommandPrefix = ""
	}
	if cfg.Platform != "discord" {
		cfg.DiscordCommandPrefix = ""
	}
}

func validatePlatformPrefixes(cfg *Config) error {
	if cfg.Platform == "slack" {
		if strings.TrimSpace(cfg.SlackCommandPrefix) == "" || strings.ContainsAny(cfg.SlackCommandPrefix, " \t\n") {
			return tmuxconn.UsageError("--slack-command-prefix must be non-empty and contain no whitespace")
		}
	}
	if cfg.Platform == "discord" {
		if strings.TrimSpace(cfg.DiscordCommandPrefix) == "" || strings.ContainsAny(cfg.DiscordCommandPrefix, " \t\n") {
			return tmuxconn.UsageError("--discord-command-prefix must be non-empty and contain no whitespace")
		}
	}
	return nil
}

func validateRunRequirements(cfg Config) error {
	registration, ok := registeredPlatform(cfg.Platform)
	if !ok {
		return tmuxconn.UsageError("%v", unsupportedPlatformError(cfg.Platform))
	}
	if registration.validateRun == nil {
		return nil
	}
	return registration.validateRun(cfg)
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
