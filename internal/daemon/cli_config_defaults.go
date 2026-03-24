package daemon

import (
	"flag"
	"os"
	"strings"
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
	defaults.whatsAppSessionDB = envOrDefault("TMUXCONN_WHATSAPP_SESSION_DB", stringValue(fileCfg.WhatsApp.SessionDB, defaultWhatsAppSessionDBPath(resolvedDBPath)))
	defaults.whatsAppDeviceName = envOrDefault("TMUXCONN_WHATSAPP_DEVICE_NAME", stringValue(fileCfg.WhatsApp.DeviceName, "tmux-connect"))
	defaults.followLines = intValue(fileCfg.FollowLines, 80)
	defaults.followMinGap, err = durationValue("[daemon].follow_min_interval", fileCfg.FollowMinInterval, 700*time.Millisecond)
	if err != nil {
		return daemonConfigDefaults{}, tmuxconn.UsageError("%v", err)
	}
	defaults.followDebug = boolValue(fileCfg.FollowDebug, false)
	if envValue, ok := envBoolValue("TMUXCONN_FOLLOW_DEBUG"); ok {
		defaults.followDebug = envValue
	}
	defaults.whatsAppAutoMarkRead = boolValue(fileCfg.WhatsApp.AutoMarkRead, true)
	if envValue, ok := envBoolValue("TMUXCONN_WHATSAPP_AUTO_MARK_READ"); ok {
		defaults.whatsAppAutoMarkRead = envValue
	}
	defaults.whatsAppAllowSelfChat = boolValue(fileCfg.WhatsApp.AllowSelfChat, false)
	if envValue, ok := envBoolValue("TMUXCONN_WHATSAPP_ALLOW_SELF_CHAT"); ok {
		defaults.whatsAppAllowSelfChat = envValue
	}
	defaults.telegramToken = envOrDefault("TMUXCONN_TELEGRAM_TOKEN", stringValue(fileCfg.Telegram.Token, ""))
	defaults.feishuAppID = envOrDefault("TMUXCONN_FEISHU_APP_ID", stringValue(fileCfg.Feishu.AppID, ""))
	defaults.feishuAppSecret = envOrDefault("TMUXCONN_FEISHU_APP_SECRET", stringValue(fileCfg.Feishu.AppSecret, ""))
	defaults.feishuBotOpenID = envOrDefault("TMUXCONN_FEISHU_BOT_OPEN_ID", stringValue(fileCfg.Feishu.BotOpenID, ""))
	defaults.feishuBotUserID = envOrDefault("TMUXCONN_FEISHU_BOT_USER_ID", stringValue(fileCfg.Feishu.BotUserID, ""))
	defaults.feishuBotUnionID = envOrDefault("TMUXCONN_FEISHU_BOT_UNION_ID", stringValue(fileCfg.Feishu.BotUnionID, ""))
	defaults.slackBotToken = envOrDefault("TMUXCONN_SLACK_BOT_TOKEN", stringValue(fileCfg.Slack.BotToken, ""))
	defaults.slackAppToken = envOrDefault("TMUXCONN_SLACK_APP_TOKEN", stringValue(fileCfg.Slack.AppToken, ""))
	defaults.slackCommandPrefix = envOrDefault("TMUXCONN_SLACK_COMMAND_PREFIX", stringValue(fileCfg.Slack.CommandPrefix, defaultSlackCommandPrefix))
	defaults.discordToken = envOrDefault("TMUXCONN_DISCORD_TOKEN", stringValue(fileCfg.Discord.Token, ""))
	defaults.discordCommandPrefix = envOrDefault("TMUXCONN_DISCORD_COMMAND_PREFIX", stringValue(fileCfg.Discord.CommandPrefix, defaultDiscordCommandPrefix))
	defaults.weixinToken = envOrDefault("TMUXCONN_WEIXIN_TOKEN", stringValue(fileCfg.Weixin.Token, ""))
	defaults.weixinBaseURL = envOrDefault("TMUXCONN_WEIXIN_BASE_URL", stringValue(fileCfg.Weixin.BaseURL, ""))
	defaults.weixinCDNBaseURL = envOrDefault("TMUXCONN_WEIXIN_CDN_BASE_URL", stringValue(fileCfg.Weixin.CDNBaseURL, ""))
	defaults.weixinRouteTag = envOrDefault("TMUXCONN_WEIXIN_ROUTE_TAG", stringValue(fileCfg.Weixin.RouteTag, ""))
	defaults.snapshotFontFile = envOrDefault("TMUXCONN_TELEGRAM_SNAPSHOT_FONT_FILE", stringValue(fileCfg.Telegram.SnapshotFontFile, ""))
	defaults.apiBaseURL = envOrDefault("TMUXCONN_TELEGRAM_API_BASE", stringValue(fileCfg.Telegram.APIBase, ""))

	return defaults, nil
}

func shouldUseWhatsAppFollowMinGap(fs *flag.FlagSet, fileCfg config.Daemon, platform string) bool {
	return platform == "whatsapp" &&
		!flagWasSet(fs, "follow-min-interval") &&
		fileCfg.FollowMinInterval == nil &&
		strings.TrimSpace(os.Getenv("TMUXCONN_FOLLOW_MIN_INTERVAL")) == ""
}
