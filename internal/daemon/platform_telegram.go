package daemon

import "io"

func init() {
	RegisterPlatformAdapter("telegram", func(cfg Config, stderr io.Writer, _ *Store) (platformAdapter, error) {
		return newTelegramAdapter(cfg, stderr), nil
	})
}
