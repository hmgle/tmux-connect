package daemon

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type IncomingMessage struct {
	Chat            ChatRef
	MessageID       string
	UserID          string
	IsFromSelf      bool
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
	return r.dispatchCommand(ctx, message, command, args)
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

func (r *Router) handleSnapshot(ctx context.Context, message IncomingMessage, args string) error {
	chat, paneKey, err := r.currentPaneForInbound(ctx, message, "command")
	if err != nil {
		return r.replyCurrentPaneError(ctx, chat, paneKey, err)
	}
	lines, mode, err := parseSnapshotArgs(args, r.snapshotLines)
	if err != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "usage", "usage: "+formatCommandUsage(r.commandPrefix(chat), "snapshot [lines] [image|text]"))
	}
	return r.replySnapshotForMode(ctx, chat, paneKey, lines, mode)
}

func (r *Router) handleSend(ctx context.Context, message IncomingMessage, args string) error {
	text := strings.TrimSpace(args)
	if text == "" {
		chat, paneKey, err := r.currentPaneForInbound(ctx, message, "command")
		if err != nil {
			return r.replyCurrentPaneError(ctx, chat, paneKey, err)
		}
		return r.promptForCommandInput(ctx, message, "send")
	}
	return r.sendText(ctx, message, text, "command")
}

func (r *Router) handlePlainText(ctx context.Context, message IncomingMessage, text string) error {
	if isWhatsAppChat(message.Chat) && message.IsFromSelf && r.plainText.WhatsAppSelfChatCommandOnly {
		r.logInboundKind(ctx, message, "", "", "input")
		return r.replyBus.Reply(ctx, message.Chat, "", "usage", "WhatsApp self-chat disables plain text to avoid reply loops. Use /send <text>, /enter <text>, /keys <key...>, or reply to a prompt.")
	}
	if r.plainText.Mode == plainTextModeExecute {
		return r.executeText(ctx, message, text, "input")
	}
	return r.sendText(ctx, message, text, "input")
}

func (r *Router) sendText(ctx context.Context, message IncomingMessage, text string, inboundKind string) error {
	chat, paneKey, err := r.currentPaneForInbound(ctx, message, inboundKind)
	if err != nil {
		return r.replyCurrentPaneError(ctx, chat, paneKey, err)
	}
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
	chat, paneKey, err := r.currentPaneForInbound(ctx, message, inboundKind)
	if err != nil {
		return r.replyCurrentPaneError(ctx, chat, paneKey, err)
	}
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
		return r.replyEnterSent(ctx, chat, paneKey)
	}
	if baselineErr != nil {
		return r.replyExecuteSnapshotError(ctx, chat, paneKey, baselineErr)
	}
	body, changed, pollErr := r.waitForExecuteSnapshot(ctx, paneKey, baseline)
	if pollErr != nil {
		return r.replyExecuteSnapshotError(ctx, chat, paneKey, pollErr)
	}
	return r.replyExecuteResult(ctx, chat, paneKey, body, changed)
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

func (r *Router) handleKeys(ctx context.Context, message IncomingMessage, args string) error {
	chat, paneKey, err := r.currentPaneForInbound(ctx, message, "command")
	if err != nil {
		return r.replyCurrentPaneError(ctx, chat, paneKey, err)
	}
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

	chat, paneKey, err := r.currentPaneForInbound(ctx, message, "command")
	if err != nil {
		return r.replyCurrentPaneError(ctx, chat, paneKey, err)
	}
	if err := r.service.EnterManaged(ctx, paneKey); err != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("enter failed: %v", err))
	}
	return r.replyBus.Reply(ctx, chat, paneKey, "enter", fmt.Sprintf("sent Enter to %s", paneKey))
}

func (r *Router) handleCtrlC(ctx context.Context, message IncomingMessage) error {
	chat, paneKey, err := r.currentPaneForInbound(ctx, message, "command")
	if err != nil {
		return r.replyCurrentPaneError(ctx, chat, paneKey, err)
	}
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
		_, paneKey, err := r.currentPaneForInbound(ctx, message, "command")
		if err != nil {
			return r.replyCurrentPaneError(ctx, chat, paneKey, err)
		}
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
