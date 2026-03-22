package daemon

import (
	"context"
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

type Config struct {
	Platform              string
	TelegramToken         string
	SlackBotToken         string
	SlackAppToken         string
	SlackCommandPrefix    string
	DiscordToken          string
	DiscordCommandPrefix  string
	WhatsAppSessionDB     string
	WhatsAppDeviceName    string
	WhatsAppAutoMarkRead  bool
	WhatsAppAllowSelfChat bool
	DBPath                string
	AllowChats            []string
	PollTimeout           time.Duration
	SnapshotLines         int
	PlainTextMode         plainTextMode
	PlainTextEcho         plainTextEchoMode
	PlainTextEchoLines    int
	PlainTextEchoDelay    time.Duration
	PlainTextEchoTimeout  time.Duration
	SnapshotTheme         string
	SnapshotFontSize      float64
	SnapshotFontFile      string
	FollowLines           int
	FollowMinGap          time.Duration
	FollowDebug           bool
	APIBaseURL            string
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

type plainTextMode string

const (
	plainTextModeType    plainTextMode = "type"
	plainTextModeExecute plainTextMode = "execute"
)

type plainTextEchoMode string

const (
	plainTextEchoOff      plainTextEchoMode = "off"
	plainTextEchoSnapshot plainTextEchoMode = "snapshot"
)

type PlainTextConfig struct {
	Mode                        plainTextMode
	Echo                        plainTextEchoMode
	EchoLines                   int
	EchoDelay                   time.Duration
	EchoTimeout                 time.Duration
	WhatsAppSelfChatCommandOnly bool
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
	router := NewRouterWithPlainTextConfig(service, registry, store, replyBus, follow, cfg.SnapshotLines, cfg.AllowChats, cfg.SlackCommandPrefix, cfg.DiscordCommandPrefix, PlainTextConfig{
		Mode:                        cfg.PlainTextMode,
		Echo:                        cfg.PlainTextEcho,
		EchoLines:                   cfg.PlainTextEchoLines,
		EchoDelay:                   cfg.PlainTextEchoDelay,
		EchoTimeout:                 cfg.PlainTextEchoTimeout,
		WhatsAppSelfChatCommandOnly: cfg.WhatsAppAllowSelfChat,
	})

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
	defaultPlainTextMode := envOrDefault("TMUXCONN_PLAIN_TEXT_MODE", stringValue(fileCfg.PlainTextMode, string(plainTextModeType)))
	defaultPlainTextEcho := envOrDefault("TMUXCONN_PLAIN_TEXT_ECHO", stringValue(fileCfg.PlainTextEcho, string(plainTextEchoSnapshot)))
	defaultPlainTextEchoLines := intValue(fileCfg.PlainTextEchoLines, 12)
	if value, ok, err := envIntValue("TMUXCONN_PLAIN_TEXT_ECHO_LINES"); err != nil {
		return Config{}, tmuxconn.UsageError("%v", err)
	} else if ok {
		defaultPlainTextEchoLines = value
	}
	defaultPlainTextEchoDelay, err := durationValue("[daemon].plain_text_echo_delay", fileCfg.PlainTextEchoDelay, 250*time.Millisecond)
	if err != nil {
		return Config{}, tmuxconn.UsageError("%v", err)
	}
	if value, ok, err := envDurationValue("TMUXCONN_PLAIN_TEXT_ECHO_DELAY", defaultPlainTextEchoDelay); err != nil {
		return Config{}, tmuxconn.UsageError("%v", err)
	} else if ok {
		defaultPlainTextEchoDelay = value
	}
	defaultPlainTextEchoTimeout, err := durationValue("[daemon].plain_text_echo_timeout", fileCfg.PlainTextEchoTimeout, 2*time.Second)
	if err != nil {
		return Config{}, tmuxconn.UsageError("%v", err)
	}
	if value, ok, err := envDurationValue("TMUXCONN_PLAIN_TEXT_ECHO_TIMEOUT", defaultPlainTextEchoTimeout); err != nil {
		return Config{}, tmuxconn.UsageError("%v", err)
	} else if ok {
		defaultPlainTextEchoTimeout = value
	}
	defaultSnapshotTheme := envOrDefault("TMUXCONN_TELEGRAM_SNAPSHOT_THEME", stringValue(fileCfg.Telegram.SnapshotTheme, termrender.ThemeDark))
	defaultWhatsAppSessionDB := envOrDefault("TMUXCONN_WHATSAPP_SESSION_DB", stringValue(fileCfg.WhatsApp.SessionDB, defaultWhatsAppSessionDBPath(resolvedDBPath)))
	defaultWhatsAppDeviceName := envOrDefault("TMUXCONN_WHATSAPP_DEVICE_NAME", stringValue(fileCfg.WhatsApp.DeviceName, "tmux-connect"))
	defaultFollowLines := intValue(fileCfg.FollowLines, 80)
	defaultFollowMinGap, err := durationValue("[daemon].follow_min_interval", fileCfg.FollowMinInterval, 700*time.Millisecond)
	if err != nil {
		return Config{}, tmuxconn.UsageError("%v", err)
	}
	defaultFollowDebug := boolValue(fileCfg.FollowDebug, false)
	if envValue, ok := envBoolValue("TMUXCONN_FOLLOW_DEBUG"); ok {
		defaultFollowDebug = envValue
	}
	defaultWhatsAppAutoMarkRead := boolValue(fileCfg.WhatsApp.AutoMarkRead, true)
	if envValue, ok := envBoolValue("TMUXCONN_WHATSAPP_AUTO_MARK_READ"); ok {
		defaultWhatsAppAutoMarkRead = envValue
	}
	defaultWhatsAppAllowSelfChat := boolValue(fileCfg.WhatsApp.AllowSelfChat, false)
	if envValue, ok := envBoolValue("TMUXCONN_WHATSAPP_ALLOW_SELF_CHAT"); ok {
		defaultWhatsAppAllowSelfChat = envValue
	}
	plainTextModeValue := defaultPlainTextMode
	plainTextEchoValue := defaultPlainTextEcho

	fs.StringVar(&cfg.Platform, "platform", defaultPlatform, "remote platform (telegram|slack|discord|whatsapp)")
	fs.StringVar(&cfg.TelegramToken, "telegram-token", envOrDefault("TMUXCONN_TELEGRAM_TOKEN", stringValue(fileCfg.Telegram.Token, "")), "telegram bot token")
	fs.StringVar(&cfg.SlackBotToken, "slack-bot-token", envOrDefault("TMUXCONN_SLACK_BOT_TOKEN", stringValue(fileCfg.Slack.BotToken, "")), "slack bot token")
	fs.StringVar(&cfg.SlackAppToken, "slack-app-token", envOrDefault("TMUXCONN_SLACK_APP_TOKEN", stringValue(fileCfg.Slack.AppToken, "")), "slack app token for socket mode")
	fs.StringVar(&cfg.SlackCommandPrefix, "slack-command-prefix", envOrDefault("TMUXCONN_SLACK_COMMAND_PREFIX", stringValue(fileCfg.Slack.CommandPrefix, defaultSlackCommandPrefix)), "command prefix for slack messages")
	fs.StringVar(&cfg.DiscordToken, "discord-token", envOrDefault("TMUXCONN_DISCORD_TOKEN", stringValue(fileCfg.Discord.Token, "")), "discord bot token")
	fs.StringVar(&cfg.DiscordCommandPrefix, "discord-command-prefix", envOrDefault("TMUXCONN_DISCORD_COMMAND_PREFIX", stringValue(fileCfg.Discord.CommandPrefix, defaultDiscordCommandPrefix)), "command prefix for discord channel messages")
	fs.StringVar(&cfg.WhatsAppSessionDB, "whatsapp-session-db", defaultWhatsAppSessionDB, "path to the WhatsApp multi-device session sqlite db")
	fs.StringVar(&cfg.WhatsAppDeviceName, "whatsapp-device-name", defaultWhatsAppDeviceName, "device name shown for WhatsApp multi-device login")
	fs.BoolVar(&cfg.WhatsAppAutoMarkRead, "whatsapp-auto-mark-read", defaultWhatsAppAutoMarkRead, "mark WhatsApp messages as read after successful handling")
	fs.BoolVar(&cfg.WhatsAppAllowSelfChat, "whatsapp-allow-self-chat", defaultWhatsAppAllowSelfChat, "allow WhatsApp self-chat commands from another linked device; plain text is disabled in this mode")
	fs.StringVar(&cfg.DBPath, "db", envOrDefault("TMUXCONN_DB_PATH", resolvedDBPath), "sqlite db path")
	fs.DurationVar(&cfg.PollTimeout, "poll-timeout", defaultPollTimeout, "telegram long polling timeout")
	fs.IntVar(&cfg.SnapshotLines, "snapshot-lines", defaultSnapshotLines, "default line count for /snapshot")
	fs.StringVar(&plainTextModeValue, "plain-text-mode", defaultPlainTextMode, "plain text behavior: type|execute")
	fs.StringVar(&plainTextEchoValue, "plain-text-echo", defaultPlainTextEcho, "plain text execute echo: off|snapshot")
	fs.IntVar(&cfg.PlainTextEchoLines, "plain-text-echo-lines", defaultPlainTextEchoLines, "line count for execute text snapshots")
	fs.DurationVar(&cfg.PlainTextEchoDelay, "plain-text-echo-delay", defaultPlainTextEchoDelay, "settle delay between execute snapshot polls")
	fs.DurationVar(&cfg.PlainTextEchoTimeout, "plain-text-echo-timeout", defaultPlainTextEchoTimeout, "maximum wait for execute output before fallback")
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
	cfg.PlainTextMode = plainTextMode(plainTextModeValue)
	cfg.PlainTextEcho = plainTextEchoMode(plainTextEchoValue)
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
	if cfg.Platform == "whatsapp" && !flagWasSet(fs, "follow-min-interval") && fileCfg.FollowMinInterval == nil && strings.TrimSpace(os.Getenv("TMUXCONN_FOLLOW_MIN_INTERVAL")) == "" {
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

func newPlatformAdapter(cfg Config, stderr io.Writer, store *Store) (platformAdapter, error) {
	switch cfg.Platform {
	case "telegram":
		return newTelegramAdapter(cfg, stderr), nil
	case "slack":
		return newSlackAdapter(cfg, stderr, store)
	case "discord":
		return newDiscordAdapter(cfg, stderr)
	case "whatsapp":
		return newWhatsAppAdapter(cfg, stderr)
	default:
		return nil, tmuxconn.UsageError("unsupported --platform %q", cfg.Platform)
	}
}
