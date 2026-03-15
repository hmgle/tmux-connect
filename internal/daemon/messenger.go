package daemon

import (
	"context"
	"strings"

	"github.com/portgle/tmux-connect/internal/telegram"
)

type messenger interface {
	SendMessage(context.Context, int64, string) (telegram.Message, error)
}

type ReplyBus struct {
	messenger messenger
	store     *Store
}

func NewReplyBus(m messenger, store *Store) *ReplyBus {
	return &ReplyBus{messenger: m, store: store}
}

func (b *ReplyBus) Reply(ctx context.Context, chatID int64, paneKey string, kind string, text string) error {
	message, err := b.messenger.SendMessage(ctx, chatID, text)
	if err != nil {
		return err
	}
	_ = b.store.LogMessage(ctx, MessageRecord{
		ChatID:            chatID,
		PaneKey:           strings.TrimSpace(paneKey),
		Direction:         "out",
		Kind:              strings.TrimSpace(kind),
		TelegramMessageID: message.MessageID,
		BodyPreview:       text,
	})
	return nil
}

func (b *ReplyBus) LogInbound(ctx context.Context, chatID int64, paneKey string, messageID int64, kind string, text string) {
	_ = b.store.LogMessage(ctx, MessageRecord{
		ChatID:            chatID,
		PaneKey:           strings.TrimSpace(paneKey),
		Direction:         "in",
		Kind:              strings.TrimSpace(kind),
		TelegramMessageID: messageID,
		BodyPreview:       text,
	})
}
