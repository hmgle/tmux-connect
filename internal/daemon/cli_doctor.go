package daemon

import (
	"context"
	"fmt"
	"io"

	"github.com/hmgle/tmux-connect/internal/config"
	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

func runDoctorWithConfig(ctx context.Context, stdout io.Writer, stderr io.Writer, service paneService, fileCfg config.Daemon, args []string) error {
	cfg, err := parseConfigWithFile(args, stderr, false, fileCfg)
	if err != nil {
		return err
	}

	fmt.Fprintln(stdout, "tmux-connect daemon doctor")

	registration, ok := registeredPlatform(cfg.Platform)
	if !ok {
		return tmuxconn.UsageError("%v", unsupportedPlatformError(cfg.Platform))
	}
	if registration.doctor != nil {
		if err := registration.doctor(stdout, cfg); err != nil {
			return err
		}
	}

	store, err := OpenStore(ctx, cfg.DBPath)
	if err != nil {
		return tmuxconn.UsageError("open sqlite store: %v", err)
	}
	defer store.Close()
	fmt.Fprintf(stdout, "sqlite store: ok (%s)\n", store.Path())

	registry := NewPaneRegistry(service)
	if err := registry.Refresh(ctx); err != nil {
		return tmuxconn.TmuxError("list panes: %v", err)
	}
	fmt.Fprintf(stdout, "tmux panes: ok (%d managed)\n", registry.ManagedCount())
	return nil
}
