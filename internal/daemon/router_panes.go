package daemon

import (
	"context"
	"fmt"
	"strings"
)

func (r *Router) handlePanes(ctx context.Context, message IncomingMessage) error {
	chat := message.Chat
	r.logInbound(ctx, message, "", "")
	if err := r.registry.Refresh(ctx); err != nil {
		return r.replyBus.Reply(ctx, chat, "", "error", fmt.Sprintf("list panes failed: %v", err))
	}
	current, err := r.store.CurrentPane(ctx, chat)
	if err != nil {
		return err
	}
	return r.replyBus.Reply(ctx, chat, "", "panes", formatPaneList(r.registry.All(), current, r.follow.IsEnabled(chat.Key())))
}

func (r *Router) handleUnmanage(ctx context.Context, message IncomingMessage, args string) error {
	chat := message.Chat
	r.logInbound(ctx, message, "", "")
	ref := strings.TrimSpace(args)
	if ref == "" {
		return r.promptForPaneCommand(ctx, message, "unmanage")
	}
	record, err := r.inspectPaneRecord(ctx, ref)
	if err != nil {
		return r.replyBus.Reply(ctx, chat, "", "error", fmt.Sprintf("inspect failed: %v", err))
	}
	paneKey := record.Info.Target.PaneKey()
	if err := r.service.Detach(ctx, ref); err != nil {
		return r.replyBus.Reply(ctx, chat, "", "error", fmt.Sprintf("unmanage failed: %v", err))
	}
	var cleanupErrs []string
	if err := r.store.UnbindPaneEverywhere(ctx, paneKey); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Sprintf("failed to clear local bindings: %v", err))
	}
	r.follow.StopPane(paneKey)
	r.registry.MarkDirty()
	if len(cleanupErrs) > 0 {
		return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("unmanaged %s but cleanup was incomplete: %s", paneKey, strings.Join(cleanupErrs, "; ")))
	}
	return r.replyBus.Reply(ctx, chat, "", "unmanage", fmt.Sprintf("unmanaged %s", paneKey))
}

func (r *Router) handleSelect(ctx context.Context, message IncomingMessage, args string) error {
	chat := message.Chat
	ref := strings.TrimSpace(args)
	if ref == "" {
		r.logInbound(ctx, message, "", "")
		return r.promptForPaneCommand(ctx, message, "select")
	}
	record, err := r.ensureManagedPaneRecord(ctx, ref)
	if err != nil {
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, chat, "", "error", err.Error())
	}
	paneKey := record.Info.Target.PaneKey()
	if err := r.store.BindPane(ctx, chat, paneKey); err != nil {
		return err
	}
	if err := r.store.SetCurrentPane(ctx, chat, paneKey); err != nil {
		return err
	}
	r.logInbound(ctx, message, paneKey, string(record.Metadata.Agent))
	if r.follow.IsEnabled(chat.Key()) {
		if err := r.follow.EnableWithOptions(ctx, chat, paneKey, r.follow.Options(chat.Key())); err != nil {
			return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("follow switch failed: %v", err))
		}
	}
	return r.replyBus.Reply(ctx, chat, paneKey, "select", fmt.Sprintf("selected %s", paneKey))
}

func (r *Router) handleClear(ctx context.Context, message IncomingMessage) error {
	chat := message.Chat
	current, err := r.store.CurrentPane(ctx, chat)
	if err != nil {
		return err
	}
	if current == "" {
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, chat, "", "clear", "no pane is currently selected")
	}
	if err := r.store.SetCurrentPane(ctx, chat, ""); err != nil {
		return err
	}
	r.follow.Disable(chat.Key())
	r.logInbound(ctx, message, current, "")
	return r.replyBus.Reply(ctx, chat, current, "clear", "cleared current pane")
}

func (r *Router) handleCurrent(ctx context.Context, message IncomingMessage) error {
	chat := message.Chat
	current, record, err := r.loadCurrentPaneRecord(ctx, chat)
	if err != nil {
		r.logInbound(ctx, message, current, "")
		return r.replyBus.Reply(ctx, chat, current, "error", err.Error())
	}
	if current == "" {
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, chat, "", "current", "no pane is currently selected")
	}
	r.logInbound(ctx, message, current, "")
	return r.replyBus.Reply(ctx, chat, current, "current", formatCurrent(record, r.follow.IsEnabled(chat.Key())))
}
