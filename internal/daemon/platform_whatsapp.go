//go:build !no_whatsapp

package daemon

import "io"

func init() {
	RegisterPlatformAdapter("whatsapp", func(cfg Config, stderr io.Writer, _ *Store) (platformAdapter, error) {
		return newWhatsAppAdapter(cfg, stderr)
	})
}
