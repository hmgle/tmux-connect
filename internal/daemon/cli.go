package daemon

import (
	"context"
	"io"
	"time"

	"github.com/hmgle/tmux-connect/internal/termrender"
	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

type Config struct {
	Platform              string
	TelegramToken         string
	FeishuAppID           string
	FeishuAppSecret       string
	FeishuBotOpenID       string
	FeishuBotUserID       string
	FeishuBotUnionID      string
	SlackBotToken         string
	SlackAppToken         string
	SlackCommandPrefix    string
	DiscordToken          string
	DiscordCommandPrefix  string
	WhatsAppSessionDB     string
	WhatsAppDeviceName    string
	WhatsAppAutoMarkRead  bool
	WhatsAppAllowSelfChat bool
	WeixinToken           string
	WeixinBaseURL         string
	WeixinCDNBaseURL      string
	WeixinRouteTag        string
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

func snapshotRenderOptions(cfg Config) termrender.Options {
	return termrender.Options{
		FontSize:  cfg.SnapshotFontSize,
		FontFile:  cfg.SnapshotFontFile,
		ThemeName: cfg.SnapshotTheme,
	}
}
