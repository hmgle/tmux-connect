//go:build !no_feishu

package daemon

import "io"

func init() {
	RegisterPlatformAdapter("feishu", func(cfg Config, stderr io.Writer, _ *Store) (platformAdapter, error) {
		return newFeishuAdapter(cfg, stderr)
	})
}
