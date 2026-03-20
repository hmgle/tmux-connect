package daemon

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hmgle/tmux-connect/internal/config"
	"github.com/hmgle/tmux-connect/internal/termrender"
	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

type Config struct {
	Platform             string
	TelegramToken        string
	SlackBotToken        string
	SlackAppToken        string
	SlackCommandPrefix   string
	DiscordToken         string
	DiscordCommandPrefix string
	DBPath               string
	AllowChats           []string
	PollTimeout          time.Duration
	SnapshotLines        int
	SnapshotTheme        string
	SnapshotFontSize     float64
	SnapshotFontFile     string
	FollowLines          int
	FollowMinGap         time.Duration
	FollowDebug          bool
	APIBaseURL           string
}

type Runtime struct {
	cfg      Config
	service  paneService
	registry *PaneRegistry
	store    *Store
	router   *Router
	follow   *FollowManager
	adapter  platformAdapter
	stderr   io.Writer
}

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

func NewRuntime(ctx context.Context, cfg Config, service paneService, stderr io.Writer) (*Runtime, error) {
	store, err := OpenStore(ctx, cfg.DBPath)
	if err != nil {
		return nil, tmuxconn.UsageError("open sqlite store: %v", err)
	}
	registry := NewPaneRegistry(service)
	if err := registry.Refresh(ctx); err != nil {
		return nil, tmuxconn.TmuxError("initial pane refresh: %v", err)
	}

	adapter, err := newPlatformAdapter(cfg, stderr, store)
	if err != nil {
		return nil, err
	}
	replyBus := NewReplyBus(adapter, store, snapshotRenderOptions(cfg))
	follow := NewFollowManager(service, replyBus, cfg.FollowLines)
	follow.minInterval = cfg.FollowMinGap
	if cfg.FollowDebug {
		follow.SetDebugWriter(stderr)
	}
	router := NewRouter(service, registry, store, replyBus, follow, cfg.SnapshotLines, cfg.AllowChats, cfg.SlackCommandPrefix, cfg.DiscordCommandPrefix)

	return &Runtime{
		cfg:      cfg,
		service:  service,
		registry: registry,
		store:    store,
		router:   router,
		follow:   follow,
		adapter:  adapter,
		stderr:   stderr,
	}, nil
}

func (r *Runtime) Run(ctx context.Context) error {
	if err := r.adapter.RegisterCommands(ctx, daemonCommandSpecs()); err != nil {
		return err
	}
	return r.adapter.Run(ctx, r.router.HandleMessage)
}

func (r *Runtime) Close() {
	if r.follow != nil {
		r.follow.Close()
	}
	if r.adapter != nil {
		_ = r.adapter.Close()
	}
}

type stringListFlag struct {
	values []string
}

func (f *stringListFlag) String() string {
	return strings.Join(f.values, ",")
}

func (f *stringListFlag) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		f.values = append(f.values, part)
	}
	return nil
}

func parseConfig(args []string, stderr io.Writer, requireRun bool) (Config, error) {
	return parseConfigWithFile(args, stderr, requireRun, config.Daemon{})
}

func parseConfigWithFile(args []string, stderr io.Writer, requireRun bool, fileCfg config.Daemon) (Config, error) {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	fs.SetOutput(stderr)

	snapshotFontSize := float64Value(fileCfg.Telegram.SnapshotFontSize, 14)
	var err error
	snapshotFontSize, err = envFloat64("TMUXCONN_TELEGRAM_SNAPSHOT_FONT_SIZE", snapshotFontSize)
	if err != nil {
		return Config{}, tmuxconn.UsageError("%v", err)
	}

	allowChats := &stringListFlag{}
	cfg := Config{}
	defaultPlatform := stringValue(fileCfg.Platform, "telegram")
	defaultPlatform = envOrDefault("TMUXCONN_PLATFORM", defaultPlatform)
	resolvedDBPath := stringValue(fileCfg.DB, defaultDBPath())
	defaultPollTimeout, err := durationValue("[daemon].poll_timeout", fileCfg.PollTimeout, 20*time.Second)
	if err != nil {
		return Config{}, tmuxconn.UsageError("%v", err)
	}
	defaultSnapshotLines := intValue(fileCfg.SnapshotLines, 120)
	defaultSnapshotTheme := envOrDefault("TMUXCONN_TELEGRAM_SNAPSHOT_THEME", stringValue(fileCfg.Telegram.SnapshotTheme, termrender.ThemeDark))
	defaultFollowLines := intValue(fileCfg.FollowLines, 80)
	defaultFollowMinGap, err := durationValue("[daemon].follow_min_interval", fileCfg.FollowMinInterval, 700*time.Millisecond)
	if err != nil {
		return Config{}, tmuxconn.UsageError("%v", err)
	}
	defaultFollowDebug := boolValue(fileCfg.FollowDebug, false)
	if envValue, ok := envBoolValue("TMUXCONN_FOLLOW_DEBUG"); ok {
		defaultFollowDebug = envValue
	}

	fs.StringVar(&cfg.Platform, "platform", defaultPlatform, "remote platform (telegram|slack|discord)")
	fs.StringVar(&cfg.TelegramToken, "telegram-token", envOrDefault("TMUXCONN_TELEGRAM_TOKEN", stringValue(fileCfg.Telegram.Token, "")), "telegram bot token")
	fs.StringVar(&cfg.SlackBotToken, "slack-bot-token", envOrDefault("TMUXCONN_SLACK_BOT_TOKEN", stringValue(fileCfg.Slack.BotToken, "")), "slack bot token")
	fs.StringVar(&cfg.SlackAppToken, "slack-app-token", envOrDefault("TMUXCONN_SLACK_APP_TOKEN", stringValue(fileCfg.Slack.AppToken, "")), "slack app token for socket mode")
	fs.StringVar(&cfg.SlackCommandPrefix, "slack-command-prefix", envOrDefault("TMUXCONN_SLACK_COMMAND_PREFIX", stringValue(fileCfg.Slack.CommandPrefix, defaultSlackCommandPrefix)), "command prefix for slack messages")
	fs.StringVar(&cfg.DiscordToken, "discord-token", envOrDefault("TMUXCONN_DISCORD_TOKEN", stringValue(fileCfg.Discord.Token, "")), "discord bot token")
	fs.StringVar(&cfg.DiscordCommandPrefix, "discord-command-prefix", envOrDefault("TMUXCONN_DISCORD_COMMAND_PREFIX", stringValue(fileCfg.Discord.CommandPrefix, defaultDiscordCommandPrefix)), "command prefix for discord channel messages")
	fs.StringVar(&cfg.DBPath, "db", envOrDefault("TMUXCONN_DB_PATH", resolvedDBPath), "sqlite db path")
	fs.DurationVar(&cfg.PollTimeout, "poll-timeout", defaultPollTimeout, "telegram long polling timeout")
	fs.IntVar(&cfg.SnapshotLines, "snapshot-lines", defaultSnapshotLines, "default line count for /snapshot")
	fs.StringVar(&cfg.SnapshotTheme, "telegram-snapshot-theme", defaultSnapshotTheme, "theme for Telegram snapshot images (dark|light)")
	fs.Float64Var(&cfg.SnapshotFontSize, "telegram-snapshot-font-size", snapshotFontSize, "font size for Telegram snapshot images")
	fs.StringVar(&cfg.SnapshotFontFile, "telegram-snapshot-font-file", envOrDefault("TMUXCONN_TELEGRAM_SNAPSHOT_FONT_FILE", stringValue(fileCfg.Telegram.SnapshotFontFile, "")), "path to a .ttf or .otf font for Telegram snapshot images")
	fs.IntVar(&cfg.FollowLines, "follow-lines", defaultFollowLines, "initial line count when starting /follow")
	fs.DurationVar(&cfg.FollowMinGap, "follow-min-interval", defaultFollowMinGap, "default minimum interval between /follow pushes")
	fs.BoolVar(&cfg.FollowDebug, "follow-debug", defaultFollowDebug, "log follow chunk/flush debug data to stderr")
	fs.StringVar(&cfg.APIBaseURL, "telegram-api-base", envOrDefault("TMUXCONN_TELEGRAM_API_BASE", stringValue(fileCfg.Telegram.APIBase, "")), "telegram bot api base url")
	fs.Var(allowChats, "allow-chat", "allowed telegram chat id (repeatable or comma-separated)")

	if err := fs.Parse(args); err != nil {
		return Config{}, tmuxconn.UsageError("%v", err)
	}
	cfg.Platform = strings.TrimSpace(strings.ToLower(cfg.Platform))
	if cfg.Platform == "" {
		cfg.Platform = "telegram"
	}
	switch {
	case flagWasSet(fs, "allow-chat"):
		cfg.AllowChats = append([]string(nil), allowChats.values...)
	case fileCfg.AllowChats != nil:
		cfg.AllowChats = append([]string(nil), (*fileCfg.AllowChats)...)
	}
	if cfg.PollTimeout <= 0 {
		return Config{}, tmuxconn.UsageError("--poll-timeout must be > 0")
	}
	if cfg.SnapshotLines <= 0 {
		return Config{}, tmuxconn.UsageError("--snapshot-lines must be > 0")
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
		default:
			return Config{}, tmuxconn.UsageError("unsupported --platform %q", cfg.Platform)
		}
	}
	if strings.TrimSpace(cfg.DBPath) == "" {
		return Config{}, tmuxconn.UsageError("--db is required")
	}
	return cfg, nil
}

func snapshotRenderOptions(cfg Config) termrender.Options {
	return termrender.Options{
		FontSize:  cfg.SnapshotFontSize,
		FontFile:  cfg.SnapshotFontFile,
		ThemeName: cfg.SnapshotTheme,
	}
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

func printUsage(w io.Writer) {
	fmt.Fprint(w, `tmux-connect daemon manages remote relay access for tmux panes.

Usage:
  tmux-connect daemon <command> [flags]

Commands:
  run      Start the relay daemon
  doctor   Validate token, sqlite store, and tmux access
  status   Show sqlite counts and current managed pane count

Common flags:
  --platform telegram|slack|discord
  --telegram-token TOKEN
  --slack-bot-token TOKEN
  --slack-app-token TOKEN
  --discord-token TOKEN
  --discord-command-prefix PREFIX
  --db PATH
  --allow-chat 123456
  --poll-timeout 20s
  --snapshot-lines 120
  --telegram-snapshot-theme dark
  --telegram-snapshot-font-size 14
  --telegram-snapshot-font-file /path/to/font.ttf
  --follow-lines 80
  --follow-min-interval 700ms
  --follow-debug
`)
}

func newPlatformAdapter(cfg Config, stderr io.Writer, store *Store) (platformAdapter, error) {
	switch cfg.Platform {
	case "telegram":
		return newTelegramAdapter(cfg, stderr), nil
	case "slack":
		return newSlackAdapter(cfg, stderr, store)
	case "discord":
		return newDiscordAdapter(cfg, stderr)
	default:
		return nil, tmuxconn.UsageError("unsupported --platform %q", cfg.Platform)
	}
}
