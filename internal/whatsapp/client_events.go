package whatsapp

import (
	"strings"
	"time"

	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
)

func (c *Client) parseMessageEvent(evt *events.Message) (MessageEvent, bool) {
	if evt == nil || evt.Message == nil {
		return MessageEvent{}, false
	}
	if evt.Info.IsGroup {
		return MessageEvent{}, false
	}
	if c.isTrackedOutboundMessage(evt.Info.ID) {
		return MessageEvent{}, false
	}
	if evt.Info.IsFromMe && !c.allowSelfChatMessage(evt) {
		return MessageEvent{}, false
	}
	text, quotedMessageID, quotedSenderID := extractText(evt.Message)
	if strings.TrimSpace(text) == "" {
		return MessageEvent{}, false
	}
	return MessageEvent{
		ChatID:          evt.Info.Chat.String(),
		SenderID:        evt.Info.Sender.String(),
		MessageID:       evt.Info.ID,
		Text:            text,
		QuotedMessageID: quotedMessageID,
		QuotedSenderID:  quotedSenderID,
		Timestamp:       evt.Info.Timestamp,
		IsFromMe:        evt.Info.IsFromMe,
		IsGroup:         evt.Info.IsGroup,
	}, true
}

func (c *Client) allowSelfChatMessage(evt *events.Message) bool {
	if !c.allowSelfChat || evt == nil {
		return false
	}
	if evt.Info.DeviceSentMeta == nil {
		return false
	}
	chatID := strings.TrimSpace(evt.Info.Chat.String())
	senderID := strings.TrimSpace(evt.Info.Sender.String())
	return chatID != "" && chatID == senderID
}

func (c *Client) trackOutboundMessage(messageID string) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return
	}
	now := time.Now()
	c.recentOutboundMu.Lock()
	defer c.recentOutboundMu.Unlock()
	if c.recentOutboundByID == nil {
		c.recentOutboundByID = make(map[string]time.Time)
	}
	c.pruneOutboundLocked(now)
	c.recentOutboundByID[messageID] = now.Add(outboundSuppressionTTL)
}

func (c *Client) isTrackedOutboundMessage(messageID string) bool {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return false
	}
	now := time.Now()
	c.recentOutboundMu.Lock()
	defer c.recentOutboundMu.Unlock()
	c.pruneOutboundLocked(now)
	expiresAt, ok := c.recentOutboundByID[messageID]
	return ok && expiresAt.After(now)
}

func (c *Client) pruneOutboundLocked(now time.Time) {
	for messageID, expiresAt := range c.recentOutboundByID {
		if !expiresAt.After(now) {
			delete(c.recentOutboundByID, messageID)
		}
	}
}

func extractText(msg *waE2E.Message) (text string, quotedMessageID string, quotedSenderID string) {
	if msg == nil {
		return "", "", ""
	}
	switch {
	case strings.TrimSpace(msg.GetConversation()) != "":
		return strings.TrimSpace(msg.GetConversation()), "", ""
	case msg.GetExtendedTextMessage() != nil:
		extended := msg.GetExtendedTextMessage()
		contextInfo := extended.GetContextInfo()
		return strings.TrimSpace(extended.GetText()), strings.TrimSpace(contextInfo.GetStanzaID()), strings.TrimSpace(contextInfo.GetParticipant())
	default:
		return "", "", ""
	}
}
