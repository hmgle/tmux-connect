package daemon

import (
	"context"
	"log"
	"strings"
)

type outboundState struct {
	paneKey          string
	sessionKey       string
	replyToMessageID string
	threadID         string
}

func (b *ReplyBus) prepareOutbound(ctx context.Context, chat ChatRef, paneKey string) outboundState {
	paneKey = strings.TrimSpace(paneKey)
	state := outboundState{paneKey: paneKey}
	if paneKey == "" {
		return state
	}
	session, err := b.store.EnsureSession(ctx, chat, paneKey, "")
	if err != nil {
		b.warnStoreError("ensure outbound session", err)
		return state
	}
	state.sessionKey = session.SessionKey
	state.replyToMessageID = session.LastInboundMessageID
	state.threadID = session.AgentThreadID
	return state
}

func (b *ReplyBus) recordOutbound(ctx context.Context, chat ChatRef, state outboundState, kind string, text string, messageID string, opts SendOptions) {
	record := MessageRecord{
		Chat:              chat,
		PaneKey:           state.paneKey,
		Direction:         "out",
		Kind:              strings.TrimSpace(kind),
		PlatformMessageID: messageID,
		ThreadID:          opts.ThreadID,
		BodyPreview:       text,
	}
	var link *MessageLinkRecord
	if state.sessionKey != "" {
		replyTarget := opts.ReplyToMessageID
		if replyTarget == "" {
			replyTarget = opts.ThreadID
		}
		link = &MessageLinkRecord{
			Platform:          chat.Platform,
			ChatID:            chat.ChatID,
			PaneKey:           state.paneKey,
			SessionKey:        state.sessionKey,
			Kind:              strings.TrimSpace(kind),
			OutboundMessageID: messageID,
			ReplyToMessageID:  replyTarget,
		}
	}
	if err := b.store.RecordOutbound(ctx, record, link); err != nil {
		b.warnStoreError("log outbound message", err)
	}
}

func (b *ReplyBus) warnStoreError(action string, err error) {
	if err == nil {
		return
	}
	log.Printf("warn: reply bus %s: %v", strings.TrimSpace(action), err)
}
