//go:build !no_slack

package daemon

import (
	"fmt"
	"io"
	"strings"

	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

func init() {
	RegisterPlatform("slack", platformRegistration{
		factory: func(cfg Config, stderr io.Writer, store *Store) (platformAdapter, error) {
			return newSlackAdapter(cfg, stderr, store)
		},
		validateRun: func(cfg Config) error {
			if strings.TrimSpace(cfg.SlackBotToken) == "" || strings.TrimSpace(cfg.SlackAppToken) == "" {
				return tmuxconn.UsageError("daemon run requires --slack-bot-token/--slack-app-token, TMUXCONN_SLACK_BOT_TOKEN/TMUXCONN_SLACK_APP_TOKEN, or [daemon.slack].bot_token/[daemon.slack].app_token in config")
			}
			return nil
		},
		doctor: func(stdout io.Writer, cfg Config) error {
			if strings.TrimSpace(cfg.SlackBotToken) == "" {
				return tmuxconn.UsageError("slack bot token is required; pass --slack-bot-token, TMUXCONN_SLACK_BOT_TOKEN, or [daemon.slack].bot_token in config")
			}
			if strings.TrimSpace(cfg.SlackAppToken) == "" {
				return tmuxconn.UsageError("slack app token is required; pass --slack-app-token, TMUXCONN_SLACK_APP_TOKEN, or [daemon.slack].app_token in config")
			}
			if _, err := fmt.Fprintln(stdout, "slack tokens: ok"); err != nil {
				return err
			}
			if _, err := fmt.Fprintln(stdout, "slack bot scopes: ensure app_mentions:read, chat:write, files:write, im:history, im:read"); err != nil {
				return err
			}
			_, err := fmt.Fprintln(stdout, "slack snapshot uploads: uses the current Web API upload flow; reinstall the app after scope changes")
			return err
		},
	})
}
