package daemon

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

func (r *Router) commandPrefix(chat ChatRef) string {
	if strings.EqualFold(strings.TrimSpace(chat.Platform), "slack") {
		return r.slackCommandPrefix
	}
	return ""
}

func (r *Router) handlePendingInput(ctx context.Context, message IncomingMessage, pending pendingCommand, args string) error {
	args = r.resolvePendingArgs(pending, args)
	return r.dispatchCommand(ctx, message, pending.Command, args)
}

func (r *Router) promptForCommandInput(ctx context.Context, message IncomingMessage, command string) error {
	spec, ok := findCommandSpec(command)
	if !ok || spec.Prompt == nil {
		return r.replyBus.Reply(ctx, message.Chat, "", "usage", "usage: "+formatCommandUsage(r.commandPrefix(message.Chat), command))
	}
	r.setPending(message.pendingKey(), pendingCommand{
		Command: spec.Command,
	})
	promptText := spec.Prompt.Message
	if strings.EqualFold(strings.TrimSpace(message.Chat.Platform), "discord") && strings.TrimSpace(message.ThreadID) != "" {
		promptText += "\n\nIn Discord channels, reply with " + strconv.Quote(r.discordCommandPrefix+" <value>") + "."
	}
	return r.replyBus.ReplyWithOptions(ctx, message.Chat, "", "prompt", promptText, r.replyBus.adapter.PromptOptions(message, *spec.Prompt))
}

func (r *Router) promptForPaneCommand(ctx context.Context, message IncomingMessage, command string) error {
	if isWhatsAppChat(message.Chat) {
		return r.promptForWhatsAppPaneChoice(ctx, message, command)
	}
	return r.promptForCommandInput(ctx, message, command)
}

func (r *Router) promptForWhatsAppPaneChoice(ctx context.Context, message IncomingMessage, command string) error {
	if err := r.registry.Refresh(ctx); err != nil {
		return r.replyBus.Reply(ctx, message.Chat, "", "error", fmt.Sprintf("list panes failed: %v", err))
	}
	records := r.registry.All()
	if len(records) == 0 {
		return r.replyBus.Reply(ctx, message.Chat, "", "panes", "No managed panes.")
	}

	options := make([]string, 0, len(records))
	lines := make([]string, 0, len(records)+2)
	switch command {
	case "select":
		lines = append(lines, "Reply with a pane number or pane id to select:")
	case "unmanage":
		lines = append(lines, "Reply with a pane number or pane id to stop managing:")
	default:
		lines = append(lines, "Reply with a pane number or pane id:")
	}
	for idx, record := range records {
		paneKey := record.Info.Target.PaneKey()
		options = append(options, paneKey)
		lines = append(lines, fmt.Sprintf("%d. %s (%s/%s)", idx+1, paneKey, record.Info.SessionName, record.Info.WindowName))
	}
	lines = append(lines, "You can also reply with %5 or default:%5 directly.")

	r.setPending(message.pendingKey(), pendingCommand{
		Command: command,
		Options: options,
	})
	return r.replyBus.ReplyWithOptions(ctx, message.Chat, "", "prompt", strings.Join(lines, "\n"), r.replyBus.adapter.PromptOptions(message, commandPromptSpec{
		Message:     lines[0],
		Placeholder: "1",
	}))
}

func (r *Router) setPending(key string, pending pendingCommand) {
	r.pendingMu.Lock()
	defer r.pendingMu.Unlock()
	r.pending[key] = pending
}

func (r *Router) consumePending(key string) (pendingCommand, bool) {
	r.pendingMu.Lock()
	defer r.pendingMu.Unlock()
	pending, ok := r.pending[key]
	if ok {
		delete(r.pending, key)
	}
	return pending, ok
}

func (r *Router) clearPending(key string) {
	r.pendingMu.Lock()
	defer r.pendingMu.Unlock()
	delete(r.pending, key)
}

func (r *Router) resolvePendingArgs(pending pendingCommand, args string) string {
	args = strings.TrimSpace(args)
	if len(pending.Options) == 0 {
		return args
	}
	index, err := strconv.Atoi(args)
	if err != nil || index <= 0 || index > len(pending.Options) {
		return args
	}
	return pending.Options[index-1]
}

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

func (r *Router) helpText(chat ChatRef) string {
	return helpTextForPlatform(chat.Platform, r.commandPrefix(chat), r.discordCommandPrefix)
}

func isWhatsAppChat(chat ChatRef) bool {
	return strings.EqualFold(strings.TrimSpace(chat.Platform), "whatsapp")
}
