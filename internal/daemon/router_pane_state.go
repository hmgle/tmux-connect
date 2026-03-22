package daemon

import (
	"context"
	"fmt"
	"strings"

	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

func (r *Router) requireCurrentPane(ctx context.Context, chat ChatRef) (string, error) {
	current, record, err := r.loadCurrentPaneRecord(ctx, chat)
	if err != nil {
		return current, err
	}
	if strings.TrimSpace(current) == "" {
		return "", fmt.Errorf("no current pane; run %s first", formatCommandUsage(r.commandPrefix(chat), "select <pane>"))
	}
	if !record.Metadata.Managed {
		_ = r.store.SetCurrentPane(ctx, chat, "")
		return current, fmt.Errorf("current pane is no longer managed")
	}
	return record.Info.Target.PaneKey(), nil
}

func (r *Router) currentPaneForInbound(ctx context.Context, message IncomingMessage, inboundKind string) (ChatRef, string, error) {
	chat := message.Chat
	paneKey, err := r.requireCurrentPane(ctx, chat)
	r.logInboundKind(ctx, message, paneKey, "", inboundKind)
	return chat, paneKey, err
}

func (r *Router) replyCurrentPaneError(ctx context.Context, chat ChatRef, paneKey string, err error) error {
	return r.replyBus.Reply(ctx, chat, paneKey, "error", err.Error())
}

func (r *Router) loadCurrentPaneRecord(ctx context.Context, chat ChatRef) (string, tmuxconn.PaneRecord, error) {
	current, err := r.store.CurrentPane(ctx, chat)
	if err != nil {
		return "", tmuxconn.PaneRecord{}, err
	}
	if strings.TrimSpace(current) == "" {
		return "", tmuxconn.PaneRecord{}, nil
	}
	record, err := r.service.Inspect(ctx, current)
	if err != nil {
		_ = r.store.SetCurrentPane(ctx, chat, "")
		return current, tmuxconn.PaneRecord{}, fmt.Errorf("current pane is unavailable: %v", err)
	}
	return current, record, nil
}

func (r *Router) inspectPaneRecord(ctx context.Context, ref string) (tmuxconn.PaneRecord, error) {
	return r.service.Inspect(ctx, ref)
}

func (r *Router) ensureManagedPaneRecord(ctx context.Context, ref string) (tmuxconn.PaneRecord, error) {
	record, err := r.inspectPaneRecord(ctx, ref)
	if err != nil {
		return tmuxconn.PaneRecord{}, fmt.Errorf("inspect failed: %v", err)
	}
	if record.Metadata.Managed {
		return record, nil
	}
	record, err = r.service.Attach(ctx, ref, "unknown", "")
	if err != nil {
		return tmuxconn.PaneRecord{}, fmt.Errorf("select failed: %v", err)
	}
	r.registry.MarkDirty()
	return record, nil
}
