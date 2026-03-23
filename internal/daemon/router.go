package daemon

import (
	"context"
	"strings"
	"sync"
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
		if shouldIgnorePlainTextMessage(message) {
			return nil
		}
		return r.handlePlainText(ctx, message, text)
	}
	if command != "" {
		r.clearPending(message.pendingKey())
	}
	return r.dispatchCommand(ctx, message, command, args)
}

func shouldIgnorePlainTextMessage(message IncomingMessage) bool {
	return isFeishuChat(message.Chat) && !isFeishuDirectMessage(message) && !message.IsAppMention
}
