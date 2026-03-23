//go:build !no_slack

package daemon

import "io"

func init() {
	RegisterPlatformAdapter("slack", func(cfg Config, stderr io.Writer, store *Store) (platformAdapter, error) {
		return newSlackAdapter(cfg, stderr, store)
	})
}
