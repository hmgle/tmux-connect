//go:build no_weixin

package daemon

import (
	"context"
	"io"

	"github.com/hmgle/tmux-connect/internal/config"
	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

func runWeixinCLIWithLoadedConfig(_ context.Context, _ io.Writer, _ io.Writer, _ config.Loaded, _ []string) error {
	return tmuxconn.UsageError("%v", unsupportedPlatformError("weixin"))
}
