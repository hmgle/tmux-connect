package daemon

import (
	"context"
	"log"
	"strings"

	"github.com/portgle/tmux-connect/internal/telegram"
	"github.com/portgle/tmux-connect/internal/termrender"
)

type messenger interface {
	SendMessage(context.Context, int64, string, telegram.SendOptions) (telegram.Message, error)
	SendPhoto(context.Context, int64, string, []byte, string, telegram.SendOptions) (telegram.Message, error)
}

type ReplyBus struct {
	messenger             messenger
	store                 *Store
	snapshotRenderOptions termrender.Options
}

const telegramPlatform = "telegram"

func NewReplyBus(m messenger, store *Store, snapshotRenderOptions termrender.Options) *ReplyBus {
	return &ReplyBus{messenger: m, store: store, snapshotRenderOptions: snapshotRenderOptions}
}

func (b *ReplyBus) Reply(ctx context.Context, chatID int64, paneKey string, kind string, text string) error {
	return b.ReplyWithOptions(ctx, chatID, paneKey, kind, text, telegram.SendOptions{})
}

func (b *ReplyBus) ReplyWithOptions(ctx context.Context, chatID int64, paneKey string, kind string, text string, opts telegram.SendOptions) error {
	state := b.prepareOutbound(ctx, chatID, paneKey)
	if opts.ReplyToMessageID <= 0 {
		opts.ReplyToMessageID = state.replyToMessageID
	}
	sendText, sendOpts := decorateTelegramMessage(kind, text, opts)
	message, err := b.messenger.SendMessage(ctx, chatID, sendText, sendOpts)
	if err != nil {
		return err
	}
	b.recordOutbound(ctx, chatID, state, kind, text, message.MessageID)
	return nil
}

func (b *ReplyBus) ReplySnapshot(ctx context.Context, chatID int64, paneKey string, text string, richText string) error {
	state := b.prepareOutbound(ctx, chatID, paneKey)
	if data, err := termrender.RenderPNG(richText, b.snapshotRenderOptions); err == nil {
		message, sendErr := b.messenger.SendPhoto(ctx, chatID, "pane-snapshot.png", data, formatSnapshotCaption(paneKey), telegram.SendOptions{
			ReplyToMessageID: state.replyToMessageID,
		})
		if sendErr == nil {
			b.recordOutbound(ctx, chatID, state, "snapshot", text, message.MessageID)
			return nil
		}
		log.Printf("warn: reply bus send snapshot photo: %v", sendErr)
	} else if strings.TrimSpace(richText) != "" {
		log.Printf("warn: reply bus render snapshot photo: %v", err)
	}

	sendText, sendOpts := decorateTelegramMessage("snapshot", formatFollowMessage(paneKey, text, 3500), telegram.SendOptions{
		ReplyToMessageID: state.replyToMessageID,
	})
	message, err := b.messenger.SendMessage(ctx, chatID, sendText, sendOpts)
	if err != nil {
		return err
	}
	b.recordOutbound(ctx, chatID, state, "snapshot", text, message.MessageID)
	return nil
}

type outboundState struct {
	paneKey          string
	sessionKey       string
	replyToMessageID int64
}

func (b *ReplyBus) prepareOutbound(ctx context.Context, chatID int64, paneKey string) outboundState {
	paneKey = strings.TrimSpace(paneKey)
	state := outboundState{paneKey: paneKey}
	if paneKey != "" {
		session, err := b.store.EnsureSession(ctx, telegramPlatform, chatID, paneKey, "")
		if err != nil {
			b.warnStoreError("ensure outbound session", err)
		} else {
			state.sessionKey = session.SessionKey
			state.replyToMessageID = session.LastInboundMessageID
		}
	}
	return state
}

func (b *ReplyBus) recordOutbound(ctx context.Context, chatID int64, state outboundState, kind string, text string, messageID int64) {
	record := MessageRecord{
		ChatID:            chatID,
		PaneKey:           state.paneKey,
		Direction:         "out",
		Kind:              strings.TrimSpace(kind),
		TelegramMessageID: messageID,
		BodyPreview:       text,
	}
	var link *MessageLinkRecord
	if state.sessionKey != "" {
		link = &MessageLinkRecord{
			Platform:          telegramPlatform,
			ChatID:            chatID,
			PaneKey:           state.paneKey,
			SessionKey:        state.sessionKey,
			Kind:              strings.TrimSpace(kind),
			OutboundMessageID: messageID,
			ReplyToMessageID:  state.replyToMessageID,
		}
	}
	if err := b.store.RecordOutbound(ctx, record, link); err != nil {
		b.warnStoreError("log outbound message", err)
	}
}

func (b *ReplyBus) LogInbound(ctx context.Context, chatID int64, paneKey string, agent string, messageID int64, kind string, text string) {
	paneKey = strings.TrimSpace(paneKey)
	if err := b.store.RecordInbound(ctx, MessageRecord{
		ChatID:            chatID,
		PaneKey:           paneKey,
		Direction:         "in",
		Kind:              strings.TrimSpace(kind),
		TelegramMessageID: messageID,
		BodyPreview:       text,
	}, telegramPlatform, agent); err != nil {
		b.warnStoreError("log inbound message", err)
	}
}

func (b *ReplyBus) warnStoreError(action string, err error) {
	if err == nil {
		return
	}
	log.Printf("warn: reply bus %s: %v", strings.TrimSpace(action), err)
}
