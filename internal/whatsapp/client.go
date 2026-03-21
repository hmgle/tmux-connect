package whatsapp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waTypes "go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

type MessageEvent struct {
	ChatID          string
	SenderID        string
	MessageID       string
	Text            string
	QuotedMessageID string
	QuotedSenderID  string
	Timestamp       time.Time
	IsFromMe        bool
	IsGroup         bool
}

type Client struct {
	client     *whatsmeow.Client
	stderr     io.Writer
	deviceName string
}

func NewClient(ctx context.Context, sessionDBPath string, deviceName string, stderr io.Writer) (*Client, error) {
	sessionDBPath = strings.TrimSpace(sessionDBPath)
	if sessionDBPath == "" {
		return nil, fmt.Errorf("whatsapp session db path is required")
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if strings.TrimSpace(deviceName) == "" {
		deviceName = "tmux-connect"
	}

	dbLog := waLog.Stdout("WhatsApp/DB", "WARN", true)
	container, err := sqlstore.New(ctx, "sqlite3", "file:"+sessionDBPath+"?_foreign_keys=on", dbLog)
	if err != nil {
		return nil, fmt.Errorf("open whatsapp session store %s: %w", sessionDBPath, err)
	}
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("get whatsapp device store: %w", err)
	}
	clientLog := waLog.Stdout("WhatsApp", "WARN", true)
	cli := whatsmeow.NewClient(deviceStore, clientLog)
	cli.SetForceActiveDeliveryReceipts(true)

	return &Client{
		client:     cli,
		stderr:     stderr,
		deviceName: deviceName,
	}, nil
}

func (c *Client) Run(ctx context.Context, autoMarkRead bool, handler func(context.Context, MessageEvent) error) error {
	if handler == nil {
		return fmt.Errorf("whatsapp handler is required")
	}

	c.client.AddEventHandler(func(evt interface{}) {
		switch event := evt.(type) {
		case *events.Message:
			msg, ok := parseMessageEvent(event)
			if !ok {
				return
			}
			go c.handleMessageEvent(ctx, autoMarkRead, msg, handler)
		}
	})

	if c.client.Store.ID == nil {
		qrChan, err := c.client.GetQRChannel(ctx)
		if err != nil {
			return fmt.Errorf("prepare whatsapp qr channel: %w", err)
		}
		go c.consumeQRChannel(qrChan)
	}

	if err := c.client.Connect(); err != nil {
		return fmt.Errorf("connect whatsapp client: %w", err)
	}
	_ = c.client.SendPresence(ctx, waTypes.PresenceAvailable)

	<-ctx.Done()
	c.client.Disconnect()
	return nil
}

func (c *Client) handleMessageEvent(ctx context.Context, autoMarkRead bool, msg MessageEvent, handler func(context.Context, MessageEvent) error) {
	if err := handler(ctx, msg); err != nil {
		fmt.Fprintf(c.stderr, "whatsapp message error: %v\n", err)
		return
	}
	if autoMarkRead {
		if err := c.markRead(ctx, msg); err != nil {
			fmt.Fprintf(c.stderr, "whatsapp mark read error: %v\n", err)
		}
	}
}

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
	return sendResp.ID, nil
}

func (c *Client) Close() error {
	if c.client == nil {
		return nil
	}
	c.client.Disconnect()
	return nil
}

func (c *Client) consumeQRChannel(ch <-chan whatsmeow.QRChannelItem) {
	for evt := range ch {
		switch evt.Event {
		case "code":
			fmt.Fprintf(c.stderr, "whatsapp login required for %s; scan this QR code from Linked Devices:\n", c.deviceName)
			qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, c.stderr)
			fmt.Fprintln(c.stderr)
		case "success":
			fmt.Fprintf(c.stderr, "whatsapp pairing completed for %s\n", c.deviceName)
		default:
			fmt.Fprintf(c.stderr, "whatsapp login event: %s\n", evt.Event)
		}
	}
}

func (c *Client) markRead(ctx context.Context, msg MessageEvent) error {
	chatJID, err := waTypes.ParseJID(msg.ChatID)
	if err != nil {
		return err
	}
	sender := waTypes.EmptyJID
	if msg.IsGroup && strings.TrimSpace(msg.SenderID) != "" {
		sender, err = waTypes.ParseJID(msg.SenderID)
		if err != nil {
			return err
		}
	}
	return c.client.MarkRead(ctx, []waTypes.MessageID{waTypes.MessageID(msg.MessageID)}, msg.Timestamp, chatJID, sender)
}

func parseMessageEvent(evt *events.Message) (MessageEvent, bool) {
	if evt == nil || evt.Message == nil {
		return MessageEvent{}, false
	}
	if evt.Info.IsFromMe || evt.Info.IsGroup {
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
