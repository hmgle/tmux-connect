package daemon

import (
	"time"

	"github.com/hmgle/tmux-connect/internal/config"
	"github.com/hmgle/tmux-connect/internal/termrender"
	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

type daemonConfigDefaults struct {
	platform              string
	telegramToken         string
	feishuAppID           string
	feishuAppSecret       string
	feishuBotOpenID       string
	feishuBotUserID       string
	feishuBotUnionID      string
	slackBotToken         string
	slackAppToken         string
	slackCommandPrefix    string
	discordToken          string
	discordCommandPrefix  string
	whatsAppSessionDB     string
	whatsAppDeviceName    string
	whatsAppAutoMarkRead  bool
	whatsAppAllowSelfChat bool
	weixinToken           string
	weixinBaseURL         string
	weixinCDNBaseURL      string
	weixinRouteTag        string
	dbPath                string
	pollTimeout           time.Duration
	snapshotLines         int
	plainTextMode         string
	plainTextEcho         string
	plainTextEchoLines    int
	plainTextEchoDelay    time.Duration
	plainTextEchoTimeout  time.Duration
	snapshotTheme         string
	snapshotFontSize      float64
	snapshotFontFile      string
	followLines           int
	followMinGap          time.Duration
	followDebug           bool
	apiBaseURL            string
}

func resolveConfigDefaults(fileCfg config.Daemon) (daemonConfigDefaults, error) {
	defaults := daemonConfigDefaults{}

	var err error
	defaults.snapshotFontSize, err = envFloat64("TMUXCONN_TELEGRAM_SNAPSHOT_FONT_SIZE", float64Value(fileCfg.Telegram.SnapshotFontSize, 14))
	if err != nil {
		return daemonConfigDefaults{}, tmuxconn.UsageError("%v", err)
	}

	defaults.platform = envOrDefault("TMUXCONN_PLATFORM", stringValue(fileCfg.Platform, defaultPlatformName()))
	resolvedDBPath := stringValue(fileCfg.DB, defaultDBPath())
	defaults.dbPath = envOrDefault("TMUXCONN_DB_PATH", resolvedDBPath)
	defaults.pollTimeout, err = durationValue("[daemon].poll_timeout", fileCfg.PollTimeout, 20*time.Second)
	if err != nil {
		return daemonConfigDefaults{}, tmuxconn.UsageError("%v", err)
	}
	defaults.snapshotLines = intValue(fileCfg.SnapshotLines, 120)
	defaults.plainTextMode = envOrDefault("TMUXCONN_PLAIN_TEXT_MODE", stringValue(fileCfg.PlainTextMode, string(plainTextModeType)))
	defaults.plainTextEcho = envOrDefault("TMUXCONN_PLAIN_TEXT_ECHO", stringValue(fileCfg.PlainTextEcho, string(plainTextEchoSnapshot)))
	defaults.plainTextEchoLines = intValue(fileCfg.PlainTextEchoLines, 12)
	if value, ok, err := envIntValue("TMUXCONN_PLAIN_TEXT_ECHO_LINES"); err != nil {
		return daemonConfigDefaults{}, tmuxconn.UsageError("%v", err)
	} else if ok {
		defaults.plainTextEchoLines = value
	}
	defaults.plainTextEchoDelay, err = durationValue("[daemon].plain_text_echo_delay", fileCfg.PlainTextEchoDelay, 250*time.Millisecond)
	if err != nil {
		return daemonConfigDefaults{}, tmuxconn.UsageError("%v", err)
	}
	if value, ok, err := envDurationValue("TMUXCONN_PLAIN_TEXT_ECHO_DELAY", defaults.plainTextEchoDelay); err != nil {
		return daemonConfigDefaults{}, tmuxconn.UsageError("%v", err)
	} else if ok {
		defaults.plainTextEchoDelay = value
	}
	defaults.plainTextEchoTimeout, err = durationValue("[daemon].plain_text_echo_timeout", fileCfg.PlainTextEchoTimeout, 2*time.Second)
	if err != nil {
		return daemonConfigDefaults{}, tmuxconn.UsageError("%v", err)
	}
	if value, ok, err := envDurationValue("TMUXCONN_PLAIN_TEXT_ECHO_TIMEOUT", defaults.plainTextEchoTimeout); err != nil {
		return daemonConfigDefaults{}, tmuxconn.UsageError("%v", err)
	} else if ok {
		defaults.plainTextEchoTimeout = value
	}
	defaults.snapshotTheme = envOrDefault("TMUXCONN_TELEGRAM_SNAPSHOT_THEME", stringValue(fileCfg.Telegram.SnapshotTheme, termrender.ThemeDark))
	defaults.followLines = intValue(fileCfg.FollowLines, 80)
	defaults.followMinGap, err = durationValue("[daemon].follow_min_interval", fileCfg.FollowMinInterval, 700*time.Millisecond)
	if err != nil {
		return daemonConfigDefaults{}, tmuxconn.UsageError("%v", err)
	}
	defaults.followDebug = boolValue(fileCfg.FollowDebug, false)
	if envValue, ok := envBoolValue("TMUXCONN_FOLLOW_DEBUG"); ok {
		defaults.followDebug = envValue
	}
	defaults.snapshotFontFile = envOrDefault("TMUXCONN_TELEGRAM_SNAPSHOT_FONT_FILE", stringValue(fileCfg.Telegram.SnapshotFontFile, ""))
	applyPlatformDefaultsFromFileAndEnv(&defaults, fileCfg, resolvedDBPath)

	return defaults, nil
}
