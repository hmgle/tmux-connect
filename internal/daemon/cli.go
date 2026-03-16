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

	"github.com/portgle/tmux-connect/internal/tagb"
	"github.com/portgle/tmux-connect/internal/telegram"
	"github.com/portgle/tmux-connect/internal/termrender"
)

type Config struct {
	TelegramToken    string
	DBPath           string
	AllowChats       []int64
	PollTimeout      time.Duration
	SnapshotLines    int
	SnapshotTheme    string
	SnapshotFontSize float64
	SnapshotFontFile string
	FollowLines      int
	FollowMinGap     time.Duration
	FollowDebug      bool
	APIBaseURL       string
}

type Runtime struct {
	cfg      Config
	service  paneService
	registry *PaneRegistry
	store    *Store
	router   *Router
	follow   *FollowManager
	client   *telegram.Client
	stderr   io.Writer
}

func RunCLI(ctx context.Context, stdout io.Writer, stderr io.Writer, service paneService, args []string) error {
	if len(args) == 0 {
		printUsage(stderr)
		return tagb.UsageError("missing daemon command")
	}
	switch args[0] {
	case "run":
		return runDaemon(ctx, stdout, stderr, service, args[1:])
	case "doctor":
		return runDoctor(ctx, stdout, stderr, service, args[1:])
	case "status":
		return runStatus(ctx, stdout, stderr, service, args[1:])
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return tagb.UsageError("unknown daemon command: %s", args[0])
	}
}

func runDaemon(ctx context.Context, stdout io.Writer, stderr io.Writer, service paneService, args []string) error {
	cfg, err := parseConfig(args, stderr, true)
	if err != nil {
		return err
	}
	runtime, err := NewRuntime(ctx, cfg, service, stderr)
	if err != nil {
		return err
	}
	defer runtime.Close()

	if _, err := fmt.Fprintf(stdout, "tagb daemon running with db %s\n", runtime.store.Path()); err != nil {
		return err
	}
	return runtime.Run(ctx)
}

func runDoctor(ctx context.Context, stdout io.Writer, stderr io.Writer, service paneService, args []string) error {
	cfg, err := parseConfig(args, stderr, false)
	if err != nil {
		return err
	}

	fmt.Fprintln(stdout, "tagb daemon doctor")

	if strings.TrimSpace(cfg.TelegramToken) == "" {
		return tagb.UsageError("telegram token is required; pass --telegram-token or TAGB_TELEGRAM_TOKEN")
	}
	fmt.Fprintln(stdout, "telegram token: ok")

	store, err := OpenStore(ctx, cfg.DBPath)
	if err != nil {
		return tagb.UsageError("open sqlite store: %v", err)
	}
	fmt.Fprintf(stdout, "sqlite store: ok (%s)\n", store.Path())

	registry := NewPaneRegistry(service)
	if err := registry.Refresh(ctx); err != nil {
		return tagb.TmuxError("list panes: %v", err)
	}
	fmt.Fprintf(stdout, "tmux panes: ok (%d managed)\n", registry.ManagedCount())
	return nil
}

func runStatus(ctx context.Context, stdout io.Writer, stderr io.Writer, service paneService, args []string) error {
	cfg, err := parseConfig(args, stderr, false)
	if err != nil {
		return err
	}
	store, err := OpenStore(ctx, cfg.DBPath)
	if err != nil {
		return tagb.UsageError("open sqlite store: %v", err)
	}
	stats, err := store.Stats(ctx)
	if err != nil {
		return tagb.UsageError("read sqlite stats: %v", err)
	}
	registry := NewPaneRegistry(service)
	refreshErr := registry.Refresh(ctx)

	fmt.Fprintln(stdout, "tagb daemon status")
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
	if strings.TrimSpace(cfg.TelegramToken) == "" {
		return nil, tagb.UsageError("telegram token is required; pass --telegram-token or TAGB_TELEGRAM_TOKEN")
	}
	store, err := OpenStore(ctx, cfg.DBPath)
	if err != nil {
		return nil, tagb.UsageError("open sqlite store: %v", err)
	}
	registry := NewPaneRegistry(service)
	if err := registry.Refresh(ctx); err != nil {
		return nil, tagb.TmuxError("initial pane refresh: %v", err)
	}

	client := telegram.NewClient(cfg.TelegramToken,
		telegram.WithBaseURL(cfg.APIBaseURL),
		telegram.WithPollTimeout(cfg.PollTimeout),
	)
	replyBus := NewReplyBus(client, store, snapshotRenderOptions(cfg))
	follow := NewFollowManager(service, replyBus, cfg.FollowLines)
	follow.minInterval = cfg.FollowMinGap
	if cfg.FollowDebug {
		follow.SetDebugWriter(stderr)
	}
	router := NewRouter(service, registry, store, replyBus, follow, cfg.SnapshotLines, cfg.AllowChats)

	return &Runtime{
		cfg:      cfg,
		service:  service,
		registry: registry,
		store:    store,
		router:   router,
		follow:   follow,
		client:   client,
		stderr:   stderr,
	}, nil
}

func (r *Runtime) Run(ctx context.Context) error {
	offset, err := r.client.DrainPendingUpdates(ctx)
	if err != nil {
		return tagb.TmuxError("drain telegram updates: %v", err)
	}
	for {
		if ctx.Err() != nil {
			return nil
		}
		updates, err := r.client.GetUpdates(ctx, offset, r.client.PollTimeout())
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			fmt.Fprintf(r.stderr, "telegram polling error: %v\n", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
			}
			continue
		}
		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			if update.Message == nil {
				continue
			}
			if strings.TrimSpace(update.Message.Text) == "" {
				continue
			}
			if err := r.router.HandleMessage(ctx, IncomingMessage{
				ChatID:    update.Message.Chat.ID,
				MessageID: update.Message.MessageID,
				Text:      update.Message.Text,
				ChatType:  update.Message.Chat.Type,
			}); err != nil {
				fmt.Fprintf(r.stderr, "router error: %v\n", err)
			}
		}
	}
}

func (r *Runtime) Close() {
	if r.follow != nil {
		r.follow.Close()
	}
}

type int64ListFlag struct {
	values []int64
}

func (f *int64ListFlag) String() string {
	out := make([]string, 0, len(f.values))
	for _, value := range f.values {
		out = append(out, strconv.FormatInt(value, 10))
	}
	return strings.Join(out, ",")
}

func (f *int64ListFlag) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parsed, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return err
		}
		f.values = append(f.values, parsed)
	}
	return nil
}

func parseConfig(args []string, stderr io.Writer, requireRun bool) (Config, error) {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	fs.SetOutput(stderr)

	snapshotFontSize, err := envFloat64("TAGB_TELEGRAM_SNAPSHOT_FONT_SIZE", 14)
	if err != nil {
		return Config{}, tagb.UsageError("%v", err)
	}

	allowChats := &int64ListFlag{}
	cfg := Config{}
	fs.StringVar(&cfg.TelegramToken, "telegram-token", strings.TrimSpace(os.Getenv("TAGB_TELEGRAM_TOKEN")), "telegram bot token")
	fs.StringVar(&cfg.DBPath, "db", envOrDefault("TAGB_DB_PATH", defaultDBPath()), "sqlite db path")
	fs.DurationVar(&cfg.PollTimeout, "poll-timeout", 20*time.Second, "telegram long polling timeout")
	fs.IntVar(&cfg.SnapshotLines, "snapshot-lines", 120, "default line count for /snapshot")
	fs.StringVar(&cfg.SnapshotTheme, "telegram-snapshot-theme", envOrDefault("TAGB_TELEGRAM_SNAPSHOT_THEME", termrender.ThemeDark), "theme for Telegram snapshot images (dark|light)")
	fs.Float64Var(&cfg.SnapshotFontSize, "telegram-snapshot-font-size", snapshotFontSize, "font size for Telegram snapshot images")
	fs.StringVar(&cfg.SnapshotFontFile, "telegram-snapshot-font-file", strings.TrimSpace(os.Getenv("TAGB_TELEGRAM_SNAPSHOT_FONT_FILE")), "path to a .ttf or .otf font for Telegram snapshot images")
	fs.IntVar(&cfg.FollowLines, "follow-lines", 80, "initial line count when starting /follow")
	fs.DurationVar(&cfg.FollowMinGap, "follow-min-interval", 700*time.Millisecond, "default minimum interval between /follow pushes")
	fs.BoolVar(&cfg.FollowDebug, "follow-debug", envBool("TAGB_FOLLOW_DEBUG"), "log follow chunk/flush debug data to stderr")
	fs.StringVar(&cfg.APIBaseURL, "telegram-api-base", strings.TrimSpace(os.Getenv("TAGB_TELEGRAM_API_BASE")), "telegram bot api base url")
	fs.Var(allowChats, "allow-chat", "allowed telegram chat id (repeatable or comma-separated)")

	if err := fs.Parse(args); err != nil {
		return Config{}, tagb.UsageError("%v", err)
	}
	cfg.AllowChats = append(cfg.AllowChats, allowChats.values...)
	if cfg.PollTimeout <= 0 {
		return Config{}, tagb.UsageError("--poll-timeout must be > 0")
	}
	if cfg.SnapshotLines <= 0 {
		return Config{}, tagb.UsageError("--snapshot-lines must be > 0")
	}
	if err := termrender.ValidateOptions(snapshotRenderOptions(cfg)); err != nil {
		return Config{}, tagb.UsageError("%v", err)
	}
	if cfg.FollowLines <= 0 {
		return Config{}, tagb.UsageError("--follow-lines must be > 0")
	}
	if cfg.FollowMinGap <= 0 {
		return Config{}, tagb.UsageError("--follow-min-interval must be > 0")
	}
	if requireRun && strings.TrimSpace(cfg.TelegramToken) == "" {
		return Config{}, tagb.UsageError("daemon run requires --telegram-token or TAGB_TELEGRAM_TOKEN")
	}
	if strings.TrimSpace(cfg.DBPath) == "" {
		return Config{}, tagb.UsageError("--db is required")
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

func envBool(key string) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `tagb daemon manages Telegram relay access for tmux panes.

Usage:
  tagb daemon <command> [flags]

Commands:
  run      Start the Telegram relay daemon
  doctor   Validate token, sqlite store, and tmux access
  status   Show sqlite counts and current managed pane count

Common flags:
  --telegram-token TOKEN
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
