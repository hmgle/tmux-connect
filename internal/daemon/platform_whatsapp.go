//go:build !no_whatsapp

package daemon

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

func init() {
	RegisterPlatform("whatsapp", platformRegistration{
		factory: func(cfg Config, stderr io.Writer, _ *Store) (platformAdapter, error) {
			return newWhatsAppAdapter(cfg, stderr)
		},
		validateRun: func(cfg Config) error {
			if strings.TrimSpace(cfg.WhatsAppSessionDB) == "" {
				return tmuxconn.UsageError("daemon run requires --whatsapp-session-db, TMUXCONN_WHATSAPP_SESSION_DB, or [daemon.whatsapp].session_db in config")
			}
			return nil
		},
		doctor: func(stdout io.Writer, cfg Config) error {
			if strings.TrimSpace(cfg.WhatsAppSessionDB) == "" {
				return tmuxconn.UsageError("whatsapp session db is required; pass --whatsapp-session-db, TMUXCONN_WHATSAPP_SESSION_DB, or [daemon.whatsapp].session_db in config")
			}
			if err := os.MkdirAll(filepath.Dir(cfg.WhatsAppSessionDB), 0o755); err != nil {
				return tmuxconn.UsageError("prepare whatsapp session db dir: %v", err)
			}
			if _, err := fmt.Fprintf(stdout, "whatsapp session db: ok (%s)\n", cfg.WhatsAppSessionDB); err != nil {
				return err
			}
			lines := []string{"whatsapp login: first run will print a pairing QR code if no device session exists"}
			if cfg.WhatsAppAllowSelfChat {
				lines = append(lines, "whatsapp self-chat: enabled (plain text is disabled; use explicit slash commands in self-chat)")
			}
			return writeDoctorLines(stdout, lines...)
		},
	})
}
