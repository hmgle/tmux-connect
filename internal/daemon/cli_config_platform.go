package daemon

import (
	"flag"
	"os"
	"strings"
	"time"

	"github.com/hmgle/tmux-connect/internal/config"
	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

func applyPlatformDefaultsFromFileAndEnv(defaults *daemonConfigDefaults, fileCfg config.Daemon, resolvedDBPath string) {
	defaults.telegramToken = envOrDefault("TMUXCONN_TELEGRAM_TOKEN", stringValue(fileCfg.Telegram.Token, ""))
	defaults.apiBaseURL = envOrDefault("TMUXCONN_TELEGRAM_API_BASE", stringValue(fileCfg.Telegram.APIBase, ""))

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

	defaults.whatsAppSessionDB = envOrDefault("TMUXCONN_WHATSAPP_SESSION_DB", stringValue(fileCfg.WhatsApp.SessionDB, defaultWhatsAppSessionDBPath(resolvedDBPath)))
	defaults.whatsAppDeviceName = envOrDefault("TMUXCONN_WHATSAPP_DEVICE_NAME", stringValue(fileCfg.WhatsApp.DeviceName, "tmux-connect"))
	defaults.whatsAppAutoMarkRead = boolValue(fileCfg.WhatsApp.AutoMarkRead, true)
	if envValue, ok := envBoolValue("TMUXCONN_WHATSAPP_AUTO_MARK_READ"); ok {
		defaults.whatsAppAutoMarkRead = envValue
	}
	defaults.whatsAppAllowSelfChat = boolValue(fileCfg.WhatsApp.AllowSelfChat, false)
	if envValue, ok := envBoolValue("TMUXCONN_WHATSAPP_ALLOW_SELF_CHAT"); ok {
		defaults.whatsAppAllowSelfChat = envValue
	}

	defaults.weixinToken = envOrDefault("TMUXCONN_WEIXIN_TOKEN", stringValue(fileCfg.Weixin.Token, ""))
	defaults.weixinBaseURL = envOrDefault("TMUXCONN_WEIXIN_BASE_URL", stringValue(fileCfg.Weixin.BaseURL, ""))
	defaults.weixinCDNBaseURL = envOrDefault("TMUXCONN_WEIXIN_CDN_BASE_URL", stringValue(fileCfg.Weixin.CDNBaseURL, ""))
	defaults.weixinRouteTag = envOrDefault("TMUXCONN_WEIXIN_ROUTE_TAG", stringValue(fileCfg.Weixin.RouteTag, ""))
}

func applyPlatformSpecificConfigDefaults(cfg *Config, fs *flag.FlagSet, fileCfg config.Daemon) {
	applyWhatsAppConfigDefaults(cfg, fs, fileCfg)
	applyWeixinConfigDefaults(cfg)
	normalizePlatformCommandPrefixes(cfg)
}

func applyWhatsAppConfigDefaults(cfg *Config, fs *flag.FlagSet, fileCfg config.Daemon) {
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
}

func applyWeixinConfigDefaults(cfg *Config) {
	if strings.TrimSpace(cfg.WeixinBaseURL) == "" {
		cfg.WeixinBaseURL = defaultWeixinBaseURL
	}
	if strings.TrimSpace(cfg.WeixinCDNBaseURL) == "" {
		cfg.WeixinCDNBaseURL = defaultWeixinCDNBaseURL
	}
	cfg.WeixinBaseURL = strings.TrimRight(strings.TrimSpace(cfg.WeixinBaseURL), "/")
	cfg.WeixinCDNBaseURL = strings.TrimRight(strings.TrimSpace(cfg.WeixinCDNBaseURL), "/")
	cfg.WeixinRouteTag = strings.TrimSpace(cfg.WeixinRouteTag)
}

func normalizePlatformCommandPrefixes(cfg *Config) {
	if cfg.Platform != "slack" {
		cfg.SlackCommandPrefix = ""
	}
	if cfg.Platform != "discord" {
		cfg.DiscordCommandPrefix = ""
	}
}

func validatePlatformPrefixes(cfg *Config) error {
	if err := validateCommandPrefix("slack", cfg.Platform, cfg.SlackCommandPrefix, "--slack-command-prefix must be non-empty and contain no whitespace"); err != nil {
		return err
	}
	if err := validateCommandPrefix("discord", cfg.Platform, cfg.DiscordCommandPrefix, "--discord-command-prefix must be non-empty and contain no whitespace"); err != nil {
		return err
	}
	return nil
}

func validateCommandPrefix(targetPlatform string, activePlatform string, prefix string, message string) error {
	if activePlatform != targetPlatform {
		return nil
	}
	if strings.TrimSpace(prefix) == "" || strings.ContainsAny(prefix, " \t\n") {
		return tmuxconn.UsageError("%s", message)
	}
	return nil
}

func shouldUseWhatsAppFollowMinGap(fs *flag.FlagSet, fileCfg config.Daemon, platform string) bool {
	return platform == "whatsapp" &&
		!flagWasSet(fs, "follow-min-interval") &&
		fileCfg.FollowMinInterval == nil &&
		strings.TrimSpace(os.Getenv("TMUXCONN_FOLLOW_MIN_INTERVAL")) == ""
}
