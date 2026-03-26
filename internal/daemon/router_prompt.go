package daemon

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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
	promptText := r.replyBus.adapter.PromptText(message, *spec.Prompt)
	return r.replyBus.ReplyWithOptions(ctx, message.Chat, "", "prompt", promptText, r.replyBus.adapter.PromptOptions(message, *spec.Prompt))
}

func (r *Router) promptForPaneCommand(ctx context.Context, message IncomingMessage, command string) error {
	if isWhatsAppChat(message.Chat) {
		return r.promptForWhatsAppPaneChoice(ctx, message, command)
	}
	if isFeishuChat(message.Chat) && (command == "select" || command == "unmanage") {
		return r.promptForFeishuPaneChoice(ctx, message, command)
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

func (r *Router) helpText(chat ChatRef) string {
	return r.replyBus.adapter.HelpText()
}

func isWhatsAppChat(chat ChatRef) bool {
	return strings.EqualFold(strings.TrimSpace(chat.Platform), "whatsapp")
}

func isFeishuChat(chat ChatRef) bool {
	return strings.EqualFold(strings.TrimSpace(chat.Platform), "feishu")
}

func isWeixinPlatform(platform string) bool {
	return strings.EqualFold(strings.TrimSpace(platform), "weixin")
}

func isWeixinChat(chat ChatRef) bool {
	return isWeixinPlatform(chat.Platform)
}

func isFeishuDirectMessage(message IncomingMessage) bool {
	return isFeishuChat(message.Chat) && strings.EqualFold(strings.TrimSpace(message.ChatType), "p2p")
}

func feishuReplyOptions(message IncomingMessage) SendOptions {
	return SendOptions{
		ReplyToMessageID: strings.TrimSpace(message.MessageID),
		ThreadID:         strings.TrimSpace(message.ThreadID),
	}
}

func (r *Router) promptForFeishuPaneChoice(ctx context.Context, message IncomingMessage, command string) error {
	if err := r.registry.Refresh(ctx); err != nil {
		return r.replyBus.Reply(ctx, message.Chat, "", "error", fmt.Sprintf("list panes failed: %v", err))
	}
	records := r.registry.All()
	if len(records) == 0 {
		return r.replyBus.Reply(ctx, message.Chat, "", "panes", "No managed panes.")
	}
	options := make([]string, 0, len(records))
	for _, record := range records {
		options = append(options, record.Info.Target.PaneKey())
	}
	r.setPending(message.pendingKey(), pendingCommand{
		Command: command,
		Options: options,
	})
	summary := "Reply with a pane number or pane id."
	if command == "unmanage" {
		summary = "Reply with a pane number or pane id to stop managing it."
	}
	opts := feishuReplyOptions(message)
	opts.Card = buildFeishuPaneChoiceCard(command, records, "")
	return r.replyBus.ReplyWithOptions(ctx, message.Chat, "", "prompt", summary, opts)
}
