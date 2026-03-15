package daemon

import (
	"context"
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
		if err == nil {
			sessionKey = session.SessionKey
			replyToMessageID, _ = b.store.LatestReplyTarget(ctx, session.SessionKey)
		}
	}

	message, err := b.messenger.SendMessage(ctx, chatID, text, telegram.SendOptions{ReplyToMessageID: replyToMessageID})
	if err != nil {
		return err
	}
	_ = b.store.LogMessage(ctx, MessageRecord{
		ChatID:            chatID,
		PaneKey:           paneKey,
		Direction:         "out",
		Kind:              strings.TrimSpace(kind),
		TelegramMessageID: message.MessageID,
		BodyPreview:       text,
	})
	if sessionKey != "" {
		_ = b.store.TouchSessionOutbound(ctx, sessionKey, message.MessageID)
		_ = b.store.CreateMessageLink(ctx, MessageLinkRecord{
			Platform:          telegramPlatform,
			ChatID:            chatID,
			PaneKey:           paneKey,
			SessionKey:        sessionKey,
			Kind:              strings.TrimSpace(kind),
			OutboundMessageID: message.MessageID,
			ReplyToMessageID:  replyToMessageID,
		})
	}
	return nil
}

func (b *ReplyBus) LogInbound(ctx context.Context, chatID int64, paneKey string, agent string, messageID int64, kind string, text string) {
	paneKey = strings.TrimSpace(paneKey)
	_ = b.store.LogMessage(ctx, MessageRecord{
		ChatID:            chatID,
		PaneKey:           paneKey,
		Direction:         "in",
		Kind:              strings.TrimSpace(kind),
		TelegramMessageID: messageID,
		BodyPreview:       text,
	})
	if paneKey == "" {
		return
	}
	session, err := b.store.EnsureSession(ctx, telegramPlatform, chatID, paneKey, agent)
	if err != nil {
		return
	}
	_ = b.store.TouchSessionInbound(ctx, session.SessionKey, messageID)
	_ = b.store.CreateMessageLink(ctx, MessageLinkRecord{
		Platform:         telegramPlatform,
		ChatID:           chatID,
		PaneKey:          paneKey,
		SessionKey:       session.SessionKey,
		Kind:             strings.TrimSpace(kind),
		InboundMessageID: messageID,
	})
}
