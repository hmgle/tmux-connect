package daemon

import (
	"context"
	"log"
	"strings"

	"github.com/hmgle/tmux-connect/internal/termrender"
)

type ReplyBus struct {
	adapter               platformAdapter
	store                 *Store
	snapshotRenderOptions termrender.Options
}

func NewReplyBus(adapter platformAdapter, store *Store, snapshotRenderOptions termrender.Options) *ReplyBus {
	return &ReplyBus{adapter: adapter, store: store, snapshotRenderOptions: snapshotRenderOptions}
}

func (b *ReplyBus) Reply(ctx context.Context, chat ChatRef, paneKey string, kind string, text string) error {
	return b.ReplyWithOptions(ctx, chat, paneKey, kind, text, SendOptions{})
}

func (b *ReplyBus) ReplyCard(ctx context.Context, chat ChatRef, paneKey string, kind string, text string, card any) error {
	return b.ReplyWithOptions(ctx, chat, paneKey, kind, text, SendOptions{Card: card})
}

func (b *ReplyBus) ReplyWithOptions(ctx context.Context, chat ChatRef, paneKey string, kind string, text string, opts SendOptions) error {
	state := b.prepareOutbound(ctx, chat, paneKey)
	opts = applyInteractionReplyContext(ctx, opts)
	if opts.ReplyToMessageID == "" {
		opts.ReplyToMessageID = state.replyToMessageID
	}
	if opts.ReplyToSenderID == "" && strings.EqualFold(chat.Platform, "whatsapp") && opts.ReplyToMessageID != "" {
		opts.ReplyToSenderID = chat.ChatID
	}
	if opts.ThreadID == "" {
		opts.ThreadID = state.threadID
	}
	sendText, sendOpts := b.adapter.DecorateMessage(kind, text, opts)
	message, err := b.adapter.SendMessage(ctx, chat, sendText, sendOpts)
	if err != nil {
		return err
	}
	b.recordOutbound(ctx, chat, state, kind, text, message.MessageID, sendOpts)
	return nil
}

func (b *ReplyBus) ReplySnapshot(ctx context.Context, chat ChatRef, paneKey string, text string, richText string) error {
	state := b.prepareOutbound(ctx, chat, paneKey)
	sendOpts := SendOptions{
		ReplyToMessageID: state.replyToMessageID,
		ReplyToSenderID:  chat.ChatID,
		ThreadID:         state.threadID,
	}
	sendOpts = applyInteractionReplyContext(ctx, sendOpts)
	if data, err := termrender.RenderPNG(richText, b.snapshotRenderOptions); err == nil {
		message, sendErr := b.adapter.SendImage(ctx, chat, "pane-snapshot.png", data, b.adapter.SnapshotCaption(paneKey), sendOpts)
		if sendErr == nil {
			b.recordOutbound(ctx, chat, state, "snapshot", text, message.MessageID, sendOpts)
			return nil
		}
		log.Printf("warn: reply bus send snapshot image: %v", sendErr)
	} else if strings.TrimSpace(richText) != "" {
		log.Printf("warn: reply bus render snapshot image: %v", err)
	}

	sendText, decoratedOpts := b.adapter.DecorateMessage("snapshot", formatFollowMessage(paneKey, text, 3500), sendOpts)
	message, err := b.adapter.SendMessage(ctx, chat, sendText, decoratedOpts)
	if err != nil {
		return err
	}
	b.recordOutbound(ctx, chat, state, "snapshot", text, message.MessageID, decoratedOpts)
	return nil
}

func (b *ReplyBus) LogInbound(ctx context.Context, message IncomingMessage, paneKey string, agent string, kind string) {
	paneKey = strings.TrimSpace(paneKey)
	if err := b.store.RecordInbound(ctx, MessageRecord{
		Chat:              message.Chat,
		PaneKey:           paneKey,
		Direction:         "in",
		Kind:              strings.TrimSpace(kind),
		PlatformMessageID: message.MessageID,
		ThreadID:          message.ThreadID,
		BodyPreview:       message.Text,
	}, message.Chat.Platform, agent); err != nil {
		b.warnStoreError("log inbound message", err)
	}
}
