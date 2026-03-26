package daemon

import (
	"context"
	"fmt"
	"io"

	"github.com/hmgle/tmux-connect/internal/config"
	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

func runStatusWithConfig(ctx context.Context, stdout io.Writer, stderr io.Writer, service paneService, fileCfg config.Daemon, args []string) error {
	cfg, err := parseConfigWithFile(args, stderr, false, fileCfg)
	if err != nil {
		return err
	}
	store, err := OpenStore(ctx, cfg.DBPath)
	if err != nil {
		return tmuxconn.UsageError("open sqlite store: %v", err)
	}
	defer store.Close()
	stats, err := store.Stats(ctx)
	if err != nil {
		return tmuxconn.UsageError("read sqlite stats: %v", err)
	}
	registry := NewPaneRegistry(service)
	refreshErr := registry.Refresh(ctx)

	fmt.Fprintln(stdout, "tmux-connect daemon status")
	fmt.Fprintf(stdout, "db: %s\n", cfg.DBPath)
	fmt.Fprintf(stdout, "registered chats: %d\n", stats.Chats)
	fmt.Fprintf(stdout, "bindings: %d\n", stats.Bindings)
	fmt.Fprintf(stdout, "message log rows: %d\n", stats.Messages)
	if refreshErr != nil {
		fmt.Fprintf(stdout, "tmux: error: %v\n", refreshErr)
		return nil
	}
	fmt.Fprintf(stdout, "managed panes: %d\n", registry.ManagedCount())
	return nil
}
