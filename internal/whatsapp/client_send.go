package whatsapp

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	waTypes "go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func (c *Client) SendText(ctx context.Context, chatID string, text string, replyToMessageID string, replyToSenderID string) (string, error) {
	chatJID, err := waTypes.ParseJID(strings.TrimSpace(chatID))
	if err != nil {
		return "", fmt.Errorf("parse whatsapp chat id %q: %w", chatID, err)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("whatsapp text is required")
	}
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        proto.String(text),
			ContextInfo: buildContextInfo(chatID, replyToMessageID, replyToSenderID),
		},
	}
	resp, err := c.client.SendMessage(ctx, chatJID, msg)
	if err != nil {
		return "", fmt.Errorf("send whatsapp text: %w", err)
	}
	c.trackOutboundMessage(resp.ID)
	return resp.ID, nil
}

func (c *Client) SendImage(ctx context.Context, chatID string, fileName string, data []byte, caption string, replyToMessageID string, replyToSenderID string) (string, error) {
	chatJID, err := waTypes.ParseJID(strings.TrimSpace(chatID))
	if err != nil {
		return "", fmt.Errorf("parse whatsapp chat id %q: %w", chatID, err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("whatsapp image data is required")
	}
	resp, err := c.client.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return "", fmt.Errorf("upload whatsapp image: %w", err)
	}
	mimeType := detectMimeType(fileName, data)
	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			Caption:       proto.String(strings.TrimSpace(caption)),
			Mimetype:      proto.String(mimeType),
			ContextInfo:   buildContextInfo(chatID, replyToMessageID, replyToSenderID),
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			MediaKey:      resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256,
			FileSHA256:    resp.FileSHA256,
			FileLength:    proto.Uint64(resp.FileLength),
		},
	}
	sendResp, err := c.client.SendMessage(ctx, chatJID, msg)
	if err != nil {
		return "", fmt.Errorf("send whatsapp image: %w", err)
	}
	c.trackOutboundMessage(sendResp.ID)
	return sendResp.ID, nil
}

func buildContextInfo(chatID string, replyToMessageID string, replyToSenderID string) *waE2E.ContextInfo {
	replyToMessageID = strings.TrimSpace(replyToMessageID)
	if replyToMessageID == "" {
		return nil
	}
	info := &waE2E.ContextInfo{
		StanzaID: proto.String(replyToMessageID),
	}
	if sender := strings.TrimSpace(replyToSenderID); sender != "" {
		info.Participant = proto.String(sender)
	}
	if chat := strings.TrimSpace(chatID); chat != "" {
		info.RemoteJID = proto.String(chat)
	}
	return info
}

func detectMimeType(fileName string, data []byte) string {
	if ext := strings.TrimSpace(filepath.Ext(fileName)); ext != "" {
		if value := mime.TypeByExtension(ext); value != "" {
			return value
		}
	}
	return http.DetectContentType(bytes.TrimSpace(data))
}
