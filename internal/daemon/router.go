package daemon

import (
	"context"
	"fmt"
	"html"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/hmgle/tmux-connect/internal/tmux"
	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

type IncomingMessage struct {
	Chat            ChatRef
	MessageID       string
	UserID          string
	Text            string
	ChatType        string
	ThreadID        string
	PendingScope    string
	IsAppMention    bool
	QuotedMessageID string
	QuotedSenderID  string
}

func (m IncomingMessage) pendingKey() string {
	scope := strings.TrimSpace(m.PendingScope)
	if scope == "" {
		scope = m.Chat.Key()
	}
	return m.Chat.Key() + "|" + scope
}

type Router struct {
	service              paneService
	registry             *PaneRegistry
	store                *Store
	replyBus             *ReplyBus
	follow               *FollowManager
	snapshotLines        int
	plainText            PlainTextConfig
	slackCommandPrefix   string
	discordCommandPrefix string
	allowChats           map[string]struct{}
	pendingMu            sync.Mutex
	pending              map[string]pendingCommand
}

type pendingCommand struct {
	Command string
	Options []string
}

func NewRouter(service paneService, registry *PaneRegistry, store *Store, replyBus *ReplyBus, follow *FollowManager, snapshotLines int, allowChats []string, slackCommandPrefix string, discordCommandPrefix string) *Router {
	return NewRouterWithPlainTextConfig(service, registry, store, replyBus, follow, snapshotLines, allowChats, slackCommandPrefix, discordCommandPrefix, PlainTextConfig{})
}

func NewRouterWithPlainTextConfig(service paneService, registry *PaneRegistry, store *Store, replyBus *ReplyBus, follow *FollowManager, snapshotLines int, allowChats []string, slackCommandPrefix string, discordCommandPrefix string, plainText PlainTextConfig) *Router {
	allowed := make(map[string]struct{}, len(allowChats))
	for _, chatID := range allowChats {
		allowed[strings.TrimSpace(chatID)] = struct{}{}
	}
	if snapshotLines <= 0 {
		snapshotLines = 120
	}
	if strings.TrimSpace(slackCommandPrefix) == "" {
		slackCommandPrefix = defaultSlackCommandPrefix
	}
	if strings.TrimSpace(discordCommandPrefix) == "" {
		discordCommandPrefix = defaultDiscordCommandPrefix
	}
	return &Router{
		service:              service,
		registry:             registry,
		store:                store,
		replyBus:             replyBus,
		follow:               follow,
		snapshotLines:        snapshotLines,
		plainText:            normalizePlainTextConfig(plainText),
		slackCommandPrefix:   slackCommandPrefix,
		discordCommandPrefix: discordCommandPrefix,
		allowChats:           allowed,
		pending:              make(map[string]pendingCommand),
	}
}

func (r *Router) commandPrefix(chat ChatRef) string {
	if strings.EqualFold(strings.TrimSpace(chat.Platform), "slack") {
		return r.slackCommandPrefix
	}
	return ""
}

func (r *Router) HandleMessage(ctx context.Context, message IncomingMessage) error {
	text := strings.TrimSpace(message.Text)
	if text == "" {
		return nil
	}

	if !r.allowed(message.Chat) {
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, message.Chat, "", "unauthorized", "chat is not allowed to use this bot")
	}

	command, args := r.parseCommand(message, text)
	if command == "" {
		if pending, ok := r.consumePending(message.pendingKey()); ok {
			return r.handlePendingInput(ctx, message, pending, text)
		}
		return r.handlePlainText(ctx, message, text)
	}
	if command != "" {
		r.clearPending(message.pendingKey())
	}

	switch command {
	case "start", "help":
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, message.Chat, "", "help", r.helpText(message.Chat))
	case "panes":
		return r.handlePanes(ctx, message)
	case "select":
		return r.handleSelect(ctx, message, args)
	case "clear":
		return r.handleClear(ctx, message)
	case "unmanage":
		return r.handleUnmanage(ctx, message, args)
	case "current":
		return r.handleCurrent(ctx, message)
	case "snapshot":
		return r.handleSnapshot(ctx, message, args)
	case "send":
		return r.handleSend(ctx, message, args)
	case "keys":
		return r.handleKeys(ctx, message, args)
	case "enter":
		return r.handleEnter(ctx, message, args)
	case "ctrlc":
		return r.handleCtrlC(ctx, message)
	case "follow":
		return r.handleFollow(ctx, message, args)
	default:
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, message.Chat, "", "unknown-command", "unknown command\n\n"+r.helpText(message.Chat))
	}
}

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
		if isWhatsAppChat(chat) {
			return r.promptForWhatsAppPaneChoice(ctx, message, "unmanage")
		}
		return r.promptForCommandInput(ctx, message, "unmanage")
	}
	record, err := r.service.Inspect(ctx, ref)
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
		if isWhatsAppChat(chat) {
			return r.promptForWhatsAppPaneChoice(ctx, message, "select")
		}
		return r.promptForCommandInput(ctx, message, "select")
	}
	record, err := r.service.Inspect(ctx, ref)
	if err != nil {
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, chat, "", "error", fmt.Sprintf("inspect failed: %v", err))
	}
	if !record.Metadata.Managed {
		record, err = r.service.Attach(ctx, ref, "unknown", "")
		if err != nil {
			r.logInbound(ctx, message, "", "")
			return r.replyBus.Reply(ctx, chat, "", "error", fmt.Sprintf("select failed: %v", err))
		}
		r.registry.MarkDirty()
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
	current, err := r.store.CurrentPane(ctx, chat)
	if err != nil {
		return err
	}
	if current == "" {
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, chat, "", "current", "no pane is currently selected")
	}
	r.logInbound(ctx, message, current, "")
	record, err := r.service.Inspect(ctx, current)
	if err != nil {
		_ = r.store.SetCurrentPane(ctx, chat, "")
		return r.replyBus.Reply(ctx, chat, current, "error", fmt.Sprintf("current pane is unavailable: %v", err))
	}
	return r.replyBus.Reply(ctx, chat, current, "current", formatCurrent(record, r.follow.IsEnabled(chat.Key())))
}

func (r *Router) handleSnapshot(ctx context.Context, message IncomingMessage, args string) error {
	chat := message.Chat
	paneKey, err := r.requireCurrentPane(ctx, chat)
	if err != nil {
		r.logInbound(ctx, message, paneKey, "")
		return r.replyBus.Reply(ctx, chat, paneKey, "error", err.Error())
	}
	r.logInbound(ctx, message, paneKey, "")
	lines, mode, err := parseSnapshotArgs(args, r.snapshotLines)
	if err != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "usage", "usage: "+formatCommandUsage(r.commandPrefix(chat), "snapshot [lines] [image|text]"))
	}
	body, err := r.service.Snapshot(ctx, paneKey, lines)
	if err != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("snapshot failed: %v", err))
	}
	if mode == snapshotModeText {
		return r.replyBus.Reply(ctx, chat, paneKey, "snapshot", formatFollowMessage(paneKey, body, 3500))
	}
	richBody, richErr := r.service.SnapshotRich(ctx, paneKey, lines)
	if richErr == nil && strings.TrimSpace(richBody) != "" {
		return r.replyBus.ReplySnapshot(ctx, chat, paneKey, body, richBody)
	}
	return r.replyBus.Reply(ctx, chat, paneKey, "snapshot", formatFollowMessage(paneKey, body, 3500))
}

func (r *Router) handleSend(ctx context.Context, message IncomingMessage, args string) error {
	text := strings.TrimSpace(args)
	if text == "" {
		chat := message.Chat
		paneKey, err := r.requireCurrentPane(ctx, chat)
		if err != nil {
			r.logInbound(ctx, message, paneKey, "")
			return r.replyBus.Reply(ctx, chat, paneKey, "error", err.Error())
		}
		r.logInbound(ctx, message, paneKey, "")
		return r.promptForCommandInput(ctx, message, "send")
	}
	return r.sendText(ctx, message, text, "command")
}

func (r *Router) handlePlainText(ctx context.Context, message IncomingMessage, text string) error {
	if r.plainText.Mode == plainTextModeExecute {
		return r.executeText(ctx, message, text, "input")
	}
	return r.sendText(ctx, message, text, "input")
}

func (r *Router) sendText(ctx context.Context, message IncomingMessage, text string, inboundKind string) error {
	chat := message.Chat
	paneKey, err := r.requireCurrentPane(ctx, chat)
	if err != nil {
		r.logInboundKind(ctx, message, paneKey, "", inboundKind)
		return r.replyBus.Reply(ctx, chat, paneKey, "error", err.Error())
	}
	r.logInboundKind(ctx, message, paneKey, "", inboundKind)
	if err := r.service.SendManaged(ctx, paneKey, text, false); err != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("send failed: %v", err))
	}
	return r.replyBus.Reply(ctx, chat, paneKey, "send", fmt.Sprintf("sent to %s", paneKey))
}

// executeText keeps the execute path local to the daemon:
//
//	baseline snapshot -> send text + Enter -> bounded poll for visible change
//	                 \-> timeout/no change => explicit fallback message
//
// "Visible change" here is intentionally heuristic. We compare plain tmux
// capture-pane text snapshots, which is good enough for relay-mode operator
// feedback but is not the same thing as shell command completion detection.
func (r *Router) executeText(ctx context.Context, message IncomingMessage, text string, inboundKind string) error {
	chat := message.Chat
	paneKey, err := r.requireCurrentPane(ctx, chat)
	if err != nil {
		r.logInboundKind(ctx, message, paneKey, "", inboundKind)
		return r.replyBus.Reply(ctx, chat, paneKey, "error", err.Error())
	}
	r.logInboundKind(ctx, message, paneKey, "", inboundKind)
	var (
		baseline    string
		baselineErr error
	)
	if r.plainText.Echo == plainTextEchoSnapshot {
		baseline, baselineErr = r.service.Snapshot(ctx, paneKey, r.plainText.EchoLines)
	}
	if err := r.service.SendManaged(ctx, paneKey, text, true); err != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("send failed: %v", err))
	}
	if r.plainText.Echo != plainTextEchoSnapshot {
		return r.replyBus.Reply(ctx, chat, paneKey, "enter", fmt.Sprintf("sent to %s and pressed Enter", paneKey))
	}
	if baselineErr != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("sent to %s and pressed Enter, but snapshot failed: %v", paneKey, baselineErr))
	}
	body, changed, pollErr := r.waitForExecuteSnapshot(ctx, paneKey, baseline)
	if pollErr != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("sent to %s and pressed Enter, but snapshot failed: %v", paneKey, pollErr))
	}
	if !changed {
		return r.replyBus.Reply(ctx, chat, paneKey, "enter", r.executeNoOutputMessage(chat, paneKey))
	}
	return r.replyBus.Reply(ctx, chat, paneKey, "snapshot", formatFollowMessage(paneKey, body, 3500))
}

func (r *Router) waitForExecuteSnapshot(ctx context.Context, paneKey string, baseline string) (string, bool, error) {
	deadline := time.Now().Add(r.plainText.EchoTimeout)
	var lastBody string
	var lastErr error

	for {
		wait := r.plainText.EchoDelay
		if remaining := time.Until(deadline); remaining <= 0 {
			break
		} else if wait > remaining {
			wait = remaining
		}
		if err := waitForDuration(ctx, wait); err != nil {
			return "", false, err
		}

		body, err := r.service.Snapshot(ctx, paneKey, r.plainText.EchoLines)
		if err != nil {
			lastErr = err
		} else {
			lastBody = body
			// Known limitation: exact string inequality is a relay-mode heuristic
			// for "something visibly changed in the pane", not a guarantee that the
			// command produced meaningful new output.
			if body != baseline {
				return body, true, nil
			}
		}
		if time.Now().After(deadline) {
			break
		}
	}

	if lastErr != nil && strings.TrimSpace(lastBody) == "" {
		return "", false, lastErr
	}
	return lastBody, false, nil
}

func waitForDuration(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (r *Router) executeNoOutputMessage(chat ChatRef, paneKey string) string {
	snapshotUsage := formatCommandUsage(r.commandPrefix(chat), "snapshot text")
	followUsage := formatCommandUsage(r.commandPrefix(chat), "follow on")
	return fmt.Sprintf("sent to %s and pressed Enter; no visible output yet. Try %s or %s.", paneKey, snapshotUsage, followUsage)
}

func (r *Router) handleKeys(ctx context.Context, message IncomingMessage, args string) error {
	chat := message.Chat
	paneKey, err := r.requireCurrentPane(ctx, chat)
	if err != nil {
		r.logInbound(ctx, message, paneKey, "")
		return r.replyBus.Reply(ctx, chat, paneKey, "error", err.Error())
	}
	r.logInbound(ctx, message, paneKey, "")
	keys, err := parseKeysArgs(args)
	if err != nil {
		if strings.TrimSpace(args) == "" {
			return r.promptForCommandInput(ctx, message, "keys")
		}
		return r.replyBus.Reply(ctx, chat, paneKey, "usage", fmt.Sprintf("%v\n\n%s", err, keysUsage(r.commandPrefix(chat))))
	}
	if err := r.service.SendKeysManaged(ctx, paneKey, keys...); err != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("send keys failed: %v", err))
	}
	return r.replyBus.Reply(ctx, chat, paneKey, "keys", fmt.Sprintf("sent keys %s to %s", strings.Join(keys, " "), paneKey))
}

func (r *Router) handleEnter(ctx context.Context, message IncomingMessage, args string) error {
	text := strings.TrimSpace(args)
	if text != "" {
		return r.executeText(ctx, message, text, "command")
	}

	chat := message.Chat
	paneKey, err := r.requireCurrentPane(ctx, chat)
	if err != nil {
		r.logInbound(ctx, message, paneKey, "")
		return r.replyBus.Reply(ctx, chat, paneKey, "error", err.Error())
	}
	r.logInbound(ctx, message, paneKey, "")
	if err := r.service.EnterManaged(ctx, paneKey); err != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("enter failed: %v", err))
	}
	return r.replyBus.Reply(ctx, chat, paneKey, "enter", fmt.Sprintf("sent Enter to %s", paneKey))
}

func (r *Router) handleCtrlC(ctx context.Context, message IncomingMessage) error {
	chat := message.Chat
	paneKey, err := r.requireCurrentPane(ctx, chat)
	if err != nil {
		r.logInbound(ctx, message, paneKey, "")
		return r.replyBus.Reply(ctx, chat, paneKey, "error", err.Error())
	}
	r.logInbound(ctx, message, paneKey, "")
	if err := r.service.CtrlCManaged(ctx, paneKey); err != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("ctrl-c failed: %v", err))
	}
	return r.replyBus.Reply(ctx, chat, paneKey, "ctrl-c", fmt.Sprintf("sent Ctrl-C to %s", paneKey))
}

func (r *Router) handleFollow(ctx context.Context, message IncomingMessage, args string) error {
	chat := message.Chat
	mode, opts, err := parseFollowArgs(args)
	if err != nil {
		if strings.TrimSpace(args) == "" {
			r.logInbound(ctx, message, "", "")
			return r.promptForCommandInput(ctx, message, "follow")
		}
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, chat, "", "usage", "usage: "+formatCommandUsage(r.commandPrefix(chat), "follow on [interval]|off"))
	}
	switch mode {
	case "on":
		paneKey, err := r.requireCurrentPane(ctx, chat)
		if err != nil {
			r.logInbound(ctx, message, paneKey, "")
			return r.replyBus.Reply(ctx, chat, paneKey, "error", err.Error())
		}
		r.logInbound(ctx, message, paneKey, "")
		if err := r.follow.EnableWithOptions(ctx, chat, paneKey, opts); err != nil {
			return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("follow failed: %v", err))
		}
		resolved := r.follow.Options(chat.Key())
		return r.replyBus.Reply(ctx, chat, paneKey, "follow", fmt.Sprintf("follow enabled for %s (min interval %s)", paneKey, resolved.MinInterval))
	case "off":
		paneKey := r.follow.CurrentPane(chat.Key())
		r.logInbound(ctx, message, paneKey, "")
		if !r.follow.Disable(chat.Key()) {
			return r.replyBus.Reply(ctx, chat, paneKey, "follow", "follow is already off")
		}
		return r.replyBus.Reply(ctx, chat, paneKey, "follow", "follow disabled")
	default:
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, chat, "", "usage", "usage: "+formatCommandUsage(r.commandPrefix(chat), "follow on [interval]|off"))
	}
}

func (r *Router) handlePendingInput(ctx context.Context, message IncomingMessage, pending pendingCommand, args string) error {
	args = r.resolvePendingArgs(pending, args)
	switch pending.Command {
	case "select":
		return r.handleSelect(ctx, message, args)
	case "unmanage":
		return r.handleUnmanage(ctx, message, args)
	case "send":
		return r.handleSend(ctx, message, args)
	case "keys":
		return r.handleKeys(ctx, message, args)
	case "follow":
		return r.handleFollow(ctx, message, args)
	default:
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, message.Chat, "", "unknown-command", "unknown command\n\n"+r.helpText(message.Chat))
	}
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
	current, err := r.store.CurrentPane(ctx, chat)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(current) == "" {
		return "", fmt.Errorf("no current pane; run %s first", formatCommandUsage(r.commandPrefix(chat), "select <pane>"))
	}
	record, err := r.service.Inspect(ctx, current)
	if err != nil {
		_ = r.store.SetCurrentPane(ctx, chat, "")
		return current, fmt.Errorf("current pane is unavailable: %w", err)
	}
	if !record.Metadata.Managed {
		_ = r.store.SetCurrentPane(ctx, chat, "")
		return current, fmt.Errorf("current pane is no longer managed")
	}
	return record.Info.Target.PaneKey(), nil
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

func (r *Router) parseCommand(message IncomingMessage, text string) (string, string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ""
	}
	if strings.HasPrefix(text, "/") {
		return parseExplicitCommand(text)
	}
	if strings.EqualFold(strings.TrimSpace(message.Chat.Platform), "slack") {
		if message.IsAppMention {
			return parseExplicitCommand(text)
		}
		if prefixed, ok := trimCommandPrefix(text, r.slackCommandPrefix); ok {
			return parseExplicitCommand(prefixed)
		}
		return "", text
	}
	return "", text
}

func parseExplicitCommand(text string) (string, string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "help", ""
	}
	command := text
	args := ""
	if idx := strings.IndexAny(text, " \n\t"); idx >= 0 {
		command = text[:idx]
		args = strings.TrimSpace(text[idx+1:])
	}
	command = normalizeCommandName(command)
	if spec, ok := findCommandSpec(command); ok {
		return spec.Command, args
	}
	return command, args
}

func trimCommandPrefix(text string, prefix string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", false
	}
	head := text
	rest := ""
	if idx := strings.IndexAny(text, " \n\t"); idx >= 0 {
		head = text[:idx]
		rest = strings.TrimSpace(text[idx+1:])
	}
	if !strings.EqualFold(head, prefix) {
		return "", false
	}
	return rest, true
}

func optionalInt(value string, fallback int) (int, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func parseKeysArgs(value string) ([]string, error) {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return nil, fmt.Errorf("missing keys")
	}
	keys := make([]string, 0, len(fields))
	for _, field := range fields {
		key, ok := normalizeTmuxKeyName(field)
		if !ok {
			return nil, fmt.Errorf("%q is not a recognized tmux key name", field)
		}
		keys = append(keys, key)
	}
	return keys, nil
}

var namedTmuxKeys = map[string]string{
	"enter":     "Enter",
	"return":    "Enter",
	"esc":       "Escape",
	"escape":    "Escape",
	"tab":       "Tab",
	"btab":      "BTab",
	"space":     "Space",
	"bspace":    "BSpace",
	"backspace": "BSpace",
	"up":        "Up",
	"down":      "Down",
	"left":      "Left",
	"right":     "Right",
	"home":      "Home",
	"end":       "End",
	"pageup":    "PageUp",
	"pgup":      "PageUp",
	"ppage":     "PageUp",
	"pagedown":  "PageDown",
	"pgdn":      "PageDown",
	"npage":     "PageDown",
	"insert":    "Insert",
	"ic":        "IC",
	"delete":    "Delete",
	"del":       "Delete",
	"dc":        "DC",
	"ctrlc":     "C-c",
	"f1":        "F1",
	"f2":        "F2",
	"f3":        "F3",
	"f4":        "F4",
	"f5":        "F5",
	"f6":        "F6",
	"f7":        "F7",
	"f8":        "F8",
	"f9":        "F9",
	"f10":       "F10",
	"f11":       "F11",
	"f12":       "F12",
	"kp0":       "KP0",
	"kp1":       "KP1",
	"kp2":       "KP2",
	"kp3":       "KP3",
	"kp4":       "KP4",
	"kp5":       "KP5",
	"kp6":       "KP6",
	"kp7":       "KP7",
	"kp8":       "KP8",
	"kp9":       "KP9",
}

func normalizeTmuxKeyName(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	if mapped, ok := namedTmuxKeys[lower]; ok {
		return mapped, true
	}
	for _, candidate := range []struct {
		prefixes []string
		modifier string
	}{
		{prefixes: []string{"ctrl-", "ctrl+", "c-"}, modifier: "C"},
		{prefixes: []string{"m-"}, modifier: "M"},
		{prefixes: []string{"s-"}, modifier: "S"},
	} {
		for _, prefix := range candidate.prefixes {
			if strings.HasPrefix(lower, prefix) {
				remainder := trimmed[len(prefix):]
				return normalizeModifiedTmuxKeyName(candidate.modifier, remainder)
			}
		}
	}
	return "", false
}

func normalizeModifiedTmuxKeyName(modifier string, remainder string) (string, bool) {
	trimmed := strings.TrimSpace(remainder)
	lower := strings.ToLower(trimmed)
	if len(lower) == 1 && lower[0] >= 'a' && lower[0] <= 'z' {
		return modifier + "-" + lower, true
	}
	if mapped, ok := namedTmuxKeys[lower]; ok {
		return modifier + "-" + mapped, true
	}
	return "", false
}

func keysUsage(commandPrefix string) string {
	return fmt.Sprintf(
		"usage: %s\n\nExamples: C-c, Enter, Tab, Escape, Up, PageUp, F1-F12, KP0-KP9\nModifiers: C-a..C-z, ctrl-c, ctrl+x, C-Space, M-x, M-Enter, S-Left",
		formatCommandUsage(commandPrefix, "keys <key...>"),
	)
}

func parseFollowArgs(value string) (string, FollowOptions, error) {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return "", FollowOptions{}, fmt.Errorf("missing follow mode")
	}

	mode := strings.ToLower(fields[0])
	switch mode {
	case "off":
		if len(fields) != 1 {
			return "", FollowOptions{}, fmt.Errorf("unexpected follow args")
		}
		return mode, FollowOptions{}, nil
	case "on":
		if len(fields) == 1 {
			return mode, FollowOptions{}, nil
		}
		if len(fields) != 2 {
			return "", FollowOptions{}, fmt.Errorf("unexpected follow args")
		}
		interval, err := parsePositiveDuration(fields[1])
		if err != nil {
			return "", FollowOptions{}, err
		}
		return mode, FollowOptions{MinInterval: interval}, nil
	default:
		return "", FollowOptions{}, fmt.Errorf("unknown follow mode")
	}
}

func parsePositiveDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("duration is required")
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0, fmt.Errorf("duration must be > 0")
		}
		return time.Duration(seconds) * time.Second, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("duration must be > 0")
	}
	return parsed, nil
}

type snapshotMode string

const (
	snapshotModeImage snapshotMode = "image"
	snapshotModeText  snapshotMode = "text"
)

func parseSnapshotArgs(value string, fallbackLines int) (int, snapshotMode, error) {
	lines := fallbackLines
	mode := snapshotModeImage
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return lines, mode, nil
	}

	for _, field := range fields {
		switch strings.ToLower(strings.TrimSpace(field)) {
		case "":
			continue
		case string(snapshotModeImage):
			mode = snapshotModeImage
		case string(snapshotModeText):
			mode = snapshotModeText
		default:
			parsed, err := strconv.Atoi(field)
			if err != nil {
				return 0, "", fmt.Errorf("invalid snapshot arg %q", field)
			}
			lines = parsed
		}
	}

	if lines <= 0 {
		return 0, "", fmt.Errorf("snapshot lines must be > 0")
	}
	return lines, mode, nil
}

func formatCurrent(record tmuxconn.PaneRecord, following bool) string {
	lines := []string{
		"Current pane: " + displayPaneKey(record.Info.Target.PaneKey()),
		"Where: " + formatPaneWhere(record.Info),
		"Command: " + displayValue(record.Info.CurrentCmd),
		"Dir: " + formatPaneDir(record.Info.CurrentPath),
		"Follow: " + onOff(following),
	}
	return strings.Join(lines, "\n")
}

func formatPaneList(records []tmuxconn.PaneRecord, current string, following bool) string {
	if len(records) == 0 {
		return "No managed panes.\n\nCurrent: none · Follow: " + onOff(following)
	}

	type row struct {
		marker string
		pane   string
		cmd    string
		dir    string
		where  string
	}

	rows := make([]row, len(records))
	header := row{marker: " ", pane: "Pane", cmd: "Cmd", dir: "Dir", where: "Where"}
	wPane := utf8.RuneCountInString(header.pane)
	wCmd := utf8.RuneCountInString(header.cmd)
	wDir := utf8.RuneCountInString(header.dir)

	for i, rec := range records {
		r := row{
			marker: " ",
			pane:   displayPaneKey(rec.Info.Target.PaneKey()),
			cmd:    shortenDisplay(displayValue(rec.Info.CurrentCmd), 16),
			dir:    formatPaneDir(rec.Info.CurrentPath),
			where:  shortenDisplay(formatPaneWhere(rec.Info), 20),
		}
		if rec.Info.Target.PaneKey() == current {
			r.marker = ">"
		}
		rows[i] = r
		if n := utf8.RuneCountInString(r.pane); n > wPane {
			wPane = n
		}
		if n := utf8.RuneCountInString(r.cmd); n > wCmd {
			wCmd = n
		}
		if n := utf8.RuneCountInString(r.dir); n > wDir {
			wDir = n
		}
	}

	var b strings.Builder
	b.WriteString("<b>Panes:</b>\n<pre>")
	writeRow := func(r row) {
		b.WriteString(r.marker)
		b.WriteByte(' ')
		writePadded(&b, r.pane, wPane)
		b.WriteString("  ")
		writePadded(&b, r.cmd, wCmd)
		b.WriteString("  ")
		writePadded(&b, r.dir, wDir)
		b.WriteString("  ")
		b.WriteString(html.EscapeString(r.where))
	}
	writeRow(header)
	for _, r := range rows {
		b.WriteByte('\n')
		writeRow(r)
	}
	b.WriteString("</pre>\nCurrent: ")
	b.WriteString(html.EscapeString(displayCurrent(current)))
	b.WriteString(" · Follow: ")
	b.WriteString(onOff(following))
	return b.String()
}

func writePadded(b *strings.Builder, s string, width int) {
	b.WriteString(html.EscapeString(s))
	for i := utf8.RuneCountInString(s); i < width; i++ {
		b.WriteByte(' ')
	}
}

func displayCurrent(current string) string {
	current = strings.TrimSpace(current)
	if current == "" {
		return "none"
	}
	return displayPaneKey(current)
}

func displayPaneKey(paneKey string) string {
	paneKey = strings.TrimSpace(paneKey)
	if paneKey == "" {
		return "-"
	}
	if strings.HasPrefix(paneKey, "default:") {
		return strings.TrimPrefix(paneKey, "default:")
	}
	return paneKey
}

func formatPaneDir(currentPath string) string {
	currentPath = strings.TrimSpace(currentPath)
	if currentPath == "" {
		return "-"
	}
	dirName := filepath.Base(filepath.Clean(currentPath))
	if strings.TrimSpace(dirName) == "" {
		dirName = currentPath
	}
	return shortenDisplay(dirName, 14)
}

func formatPaneWhere(info tmux.PaneInfo) string {
	return strings.TrimSpace(displayValue(info.SessionName) + "/" + displayValue(info.WindowName))
}

func displayValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func shortenDisplay(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	runes := []rune(value)
	if len(runes) <= maxRunes || maxRunes <= 0 {
		return value
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	remaining := maxRunes - 3
	prefix := remaining / 2
	suffix := remaining - prefix
	if prefix == 0 {
		return "..." + string(runes[len(runes)-suffix:])
	}
	if suffix == 0 {
		return string(runes[:prefix]) + "..."
	}
	return string(runes[:prefix]) + "..." + string(runes[len(runes)-suffix:])
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}
