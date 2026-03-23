//go:build !no_discord

package daemon

import "io"

func init() {
	RegisterPlatformAdapter("discord", func(cfg Config, stderr io.Writer, _ *Store) (platformAdapter, error) {
		return newDiscordAdapter(cfg, stderr)
	})
}
