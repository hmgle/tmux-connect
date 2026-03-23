//go:build !no_discord

package daemon

import (
	"fmt"
	"io"
	"strings"

	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

func init() {
	RegisterPlatform("discord", platformRegistration{
		factory: func(cfg Config, stderr io.Writer, _ *Store) (platformAdapter, error) {
			return newDiscordAdapter(cfg, stderr)
		},
		validateRun: func(cfg Config) error {
			if strings.TrimSpace(cfg.DiscordToken) == "" {
				return tmuxconn.UsageError("daemon run requires --discord-token, TMUXCONN_DISCORD_TOKEN, or [daemon.discord].token in config")
			}
			return nil
		},
		doctor: func(stdout io.Writer, cfg Config) error {
			if strings.TrimSpace(cfg.DiscordToken) == "" {
				return tmuxconn.UsageError("discord token is required; pass --discord-token, TMUXCONN_DISCORD_TOKEN, or [daemon.discord].token in config")
			}
			if _, err := fmt.Fprintln(stdout, "discord token: ok"); err != nil {
				return err
			}
			_, err := fmt.Fprintln(stdout, "discord gateway intents: enable Message Content intent for prefix commands and DMs")
			return err
		},
	})
}
