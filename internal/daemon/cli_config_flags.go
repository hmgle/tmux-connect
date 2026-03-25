package daemon

import (
	"flag"
	"strings"
)

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

type daemonConfigFlagValues struct {
	allowChats    *stringListFlag
	plainTextMode string
	plainTextEcho string
}

func bindConfigFlags(fs *flag.FlagSet, cfg *Config, defaults daemonConfigDefaults) *daemonConfigFlagValues {
	values := &daemonConfigFlagValues{
		allowChats:    &stringListFlag{},
		plainTextMode: defaults.plainTextMode,
		plainTextEcho: defaults.plainTextEcho,
	}

	platformChoices := availablePlatformChoices()
	if platformChoices == "" {
		platformChoices = "none"
	}

	fs.StringVar(&cfg.Platform, "platform", defaults.platform, "remote platform in this build ("+platformChoices+")")
	fs.StringVar(&cfg.TelegramToken, "telegram-token", defaults.telegramToken, "telegram bot token")
	fs.StringVar(&cfg.FeishuAppID, "feishu-app-id", defaults.feishuAppID, "feishu app id")
	fs.StringVar(&cfg.FeishuAppSecret, "feishu-app-secret", defaults.feishuAppSecret, "feishu app secret")
	fs.StringVar(&cfg.FeishuBotOpenID, "feishu-bot-open-id", defaults.feishuBotOpenID, "feishu bot open_id for precise group @mention detection")
	fs.StringVar(&cfg.FeishuBotUserID, "feishu-bot-user-id", defaults.feishuBotUserID, "feishu bot user_id for precise group @mention detection")
	fs.StringVar(&cfg.FeishuBotUnionID, "feishu-bot-union-id", defaults.feishuBotUnionID, "feishu bot union_id for precise group @mention detection")
	fs.StringVar(&cfg.SlackBotToken, "slack-bot-token", defaults.slackBotToken, "slack bot token")
	fs.StringVar(&cfg.SlackAppToken, "slack-app-token", defaults.slackAppToken, "slack app token for socket mode")
	fs.StringVar(&cfg.SlackCommandPrefix, "slack-command-prefix", defaults.slackCommandPrefix, "command prefix for slack messages")
	fs.StringVar(&cfg.DiscordToken, "discord-token", defaults.discordToken, "discord bot token")
	fs.StringVar(&cfg.DiscordCommandPrefix, "discord-command-prefix", defaults.discordCommandPrefix, "command prefix for discord channel messages")
	fs.StringVar(&cfg.WhatsAppSessionDB, "whatsapp-session-db", defaults.whatsAppSessionDB, "path to the WhatsApp multi-device session sqlite db")
	fs.StringVar(&cfg.WhatsAppDeviceName, "whatsapp-device-name", defaults.whatsAppDeviceName, "device name shown for WhatsApp multi-device login")
	fs.BoolVar(&cfg.WhatsAppAutoMarkRead, "whatsapp-auto-mark-read", defaults.whatsAppAutoMarkRead, "mark WhatsApp messages as read after successful handling")
	fs.BoolVar(&cfg.WhatsAppAllowSelfChat, "whatsapp-allow-self-chat", defaults.whatsAppAllowSelfChat, "allow WhatsApp self-chat commands from another linked device; plain text is disabled in this mode")
	fs.StringVar(&cfg.WeixinToken, "weixin-token", defaults.weixinToken, "weixin ilink bot bearer token")
	fs.StringVar(&cfg.WeixinBaseURL, "weixin-base-url", defaults.weixinBaseURL, "weixin ilink api base url")
	fs.StringVar(&cfg.WeixinCDNBaseURL, "weixin-cdn-base-url", defaults.weixinCDNBaseURL, "weixin ilink cdn base url")
	fs.StringVar(&cfg.WeixinRouteTag, "weixin-route-tag", defaults.weixinRouteTag, "optional weixin ilink SKRouteTag header")
	fs.StringVar(&cfg.DBPath, "db", defaults.dbPath, "sqlite db path")
	fs.DurationVar(&cfg.PollTimeout, "poll-timeout", defaults.pollTimeout, "telegram long polling timeout")
	fs.IntVar(&cfg.SnapshotLines, "snapshot-lines", defaults.snapshotLines, "default line count for /snapshot")
	fs.StringVar(&values.plainTextMode, "plain-text-mode", defaults.plainTextMode, "plain text behavior: type|execute")
	fs.StringVar(&values.plainTextEcho, "plain-text-echo", defaults.plainTextEcho, "plain text execute echo: off|snapshot")
	fs.IntVar(&cfg.PlainTextEchoLines, "plain-text-echo-lines", defaults.plainTextEchoLines, "line count for execute text snapshots")
	fs.DurationVar(&cfg.PlainTextEchoDelay, "plain-text-echo-delay", defaults.plainTextEchoDelay, "settle delay between execute snapshot polls")
	fs.DurationVar(&cfg.PlainTextEchoTimeout, "plain-text-echo-timeout", defaults.plainTextEchoTimeout, "maximum wait for execute output before fallback")
	fs.StringVar(&cfg.SnapshotTheme, "telegram-snapshot-theme", defaults.snapshotTheme, "theme for Telegram snapshot images (dark|light)")
	fs.Float64Var(&cfg.SnapshotFontSize, "telegram-snapshot-font-size", defaults.snapshotFontSize, "font size for Telegram snapshot images")
	fs.StringVar(&cfg.SnapshotFontFile, "telegram-snapshot-font-file", defaults.snapshotFontFile, "path to a .ttf or .otf font for Telegram snapshot images")
	fs.IntVar(&cfg.FollowLines, "follow-lines", defaults.followLines, "initial line count when starting /follow")
	fs.DurationVar(&cfg.FollowMinGap, "follow-min-interval", defaults.followMinGap, "default minimum interval between /follow pushes")
	fs.BoolVar(&cfg.FollowDebug, "follow-debug", defaults.followDebug, "log follow chunk/flush debug data to stderr")
	fs.StringVar(&cfg.APIBaseURL, "telegram-api-base", defaults.apiBaseURL, "telegram bot api base url")
	fs.Var(values.allowChats, "allow-chat", "allowed remote chat id (repeatable or comma-separated, preferably platform-scoped like feishu:oc_xxx)")

	return values
}
