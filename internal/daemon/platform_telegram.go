//go:build !no_telegram

package daemon

import (
	"fmt"
	"io"
	"strings"

	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

func init() {
	RegisterPlatform("telegram", platformRegistration{
		factory: func(cfg Config, stderr io.Writer, _ *Store) (platformAdapter, error) {
			return newTelegramAdapter(cfg, stderr), nil
		},
		validateRun: func(cfg Config) error {
			if strings.TrimSpace(cfg.TelegramToken) == "" {
				return tmuxconn.UsageError("daemon run requires --telegram-token, TMUXCONN_TELEGRAM_TOKEN, or [daemon.telegram].token in config")
			}
			return nil
		},
		doctor: func(stdout io.Writer, cfg Config) error {
			if strings.TrimSpace(cfg.TelegramToken) == "" {
				return tmuxconn.UsageError("telegram token is required; pass --telegram-token, TMUXCONN_TELEGRAM_TOKEN, or [daemon.telegram].token in config")
			}
			_, err := fmt.Fprintln(stdout, "telegram token: ok")
			return err
		},
	})
}
