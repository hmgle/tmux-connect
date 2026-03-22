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

func (r *Router) enableFollow(ctx context.Context, message IncomingMessage, opts FollowOptions) error {
	chat, paneKey, err := r.currentPaneForInbound(ctx, message, "command")
	if err != nil {
		return r.replyCurrentPaneError(ctx, chat, paneKey, err)
	}
	if err := r.follow.EnableWithOptions(ctx, chat, paneKey, opts); err != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("follow failed: %v", err))
	}
	return r.replyFollowEnabled(ctx, chat, paneKey, r.follow.Options(chat.Key()))
}

func (r *Router) disableFollow(ctx context.Context, message IncomingMessage) error {
	chat := message.Chat
	paneKey := r.follow.CurrentPane(chat.Key())
	r.logInbound(ctx, message, paneKey, "")
	return r.replyFollowDisabled(ctx, chat, paneKey, r.follow.Disable(chat.Key()))
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

func (r *Router) logInbound(ctx context.Context, message IncomingMessage, paneKey string, agent string) {
	r.logInboundKind(ctx, message, paneKey, agent, "command")
}

func (r *Router) logInboundKind(ctx context.Context, message IncomingMessage, paneKey string, agent string, kind string) {
	r.replyBus.LogInbound(ctx, message, paneKey, agent, kind)
}

func (r *Router) allowed(chat ChatRef) bool {
	if len(r.allowChats) == 0 {
		return true
	}
	_, ok := r.allowChats[chat.Key()]
	if ok {
		return true
	}
	_, ok = r.allowChats[chat.ChatID]
	return ok
}
