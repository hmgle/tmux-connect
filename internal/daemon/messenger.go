package daemon

import (
	"context"
	"log"
	"strings"

	"github.com/portgle/tmux-connect/internal/telegram"
)

type messenger interface {
	SendMessage(context.Context, int64, string, telegram.SendOptions) (telegram.Message, error)
}

type ReplyBus struct {
	messenger messenger
	store     *Store
}

const telegramPlatform = "telegram"

func NewReplyBus(m messenger, store *Store) *ReplyBus {
	return &ReplyBus{messenger: m, store: store}
}

func (b *ReplyBus) Reply(ctx context.Context, chatID int64, paneKey string, kind string, text string) error {
	paneKey = strings.TrimSpace(paneKey)
	sessionKey := ""
	replyToMessageID := int64(0)
	if paneKey != "" {
		session, err := b.store.EnsureSession(ctx, telegramPlatform, chatID, paneKey, "")
		if err != nil {
			b.warnStoreError("ensure outbound session", err)
		} else {
			sessionKey = session.SessionKey
			replyToMessageID = session.LastInboundMessageID
		}
	}

	sendText, sendOpts := decorateTelegramMessage(kind, text, telegram.SendOptions{ReplyToMessageID: replyToMessageID})
	message, err := b.messenger.SendMessage(ctx, chatID, sendText, sendOpts)
	if err != nil {
		return err
	}
	if err := b.store.LogMessage(ctx, MessageRecord{
		ChatID:            chatID,
		PaneKey:           paneKey,
		Direction:         "out",
		Kind:              strings.TrimSpace(kind),
		TelegramMessageID: message.MessageID,
		BodyPreview:       text,
	}); err != nil {
		b.warnStoreError("log outbound message", err)
	}
	if sessionKey != "" {
		if err := b.store.TouchSessionOutbound(ctx, sessionKey, message.MessageID); err != nil {
			b.warnStoreError("touch outbound session", err)
		}
		if err := b.store.CreateMessageLink(ctx, MessageLinkRecord{
			Platform:          telegramPlatform,
			ChatID:            chatID,
			PaneKey:           paneKey,
			SessionKey:        sessionKey,
			Kind:              strings.TrimSpace(kind),
			OutboundMessageID: message.MessageID,
			ReplyToMessageID:  replyToMessageID,
		}); err != nil {
			b.warnStoreError("create outbound message link", err)
		}
	}
	return nil
}

func (b *ReplyBus) LogInbound(ctx context.Context, chatID int64, paneKey string, agent string, messageID int64, kind string, text string) {
	paneKey = strings.TrimSpace(paneKey)
	if err := b.store.LogMessage(ctx, MessageRecord{
		ChatID:            chatID,
		PaneKey:           paneKey,
		Direction:         "in",
		Kind:              strings.TrimSpace(kind),
		TelegramMessageID: messageID,
		BodyPreview:       text,
	}); err != nil {
		b.warnStoreError("log inbound message", err)
	}
	if paneKey == "" {
		return
	}
	session, err := b.store.EnsureSession(ctx, telegramPlatform, chatID, paneKey, agent)
	if err != nil {
		b.warnStoreError("ensure inbound session", err)
		return
	}
	if err := b.store.TouchSessionInbound(ctx, session.SessionKey, messageID); err != nil {
		b.warnStoreError("touch inbound session", err)
	}
	if err := b.store.CreateMessageLink(ctx, MessageLinkRecord{
		Platform:         telegramPlatform,
		ChatID:           chatID,
		PaneKey:          paneKey,
		SessionKey:       session.SessionKey,
		Kind:             strings.TrimSpace(kind),
		InboundMessageID: messageID,
	}); err != nil {
		b.warnStoreError("create inbound message link", err)
	}
}

func (b *ReplyBus) warnStoreError(action string, err error) {
	if err == nil {
		return
	}
	log.Printf("warn: reply bus %s: %v", strings.TrimSpace(action), err)
}
