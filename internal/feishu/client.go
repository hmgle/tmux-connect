package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkdispatcher "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

const (
	receiveIDTypeChatID = "chat_id"
	messageTypeText     = "text"
	messageTypeImage    = "image"
	messageTypeCard     = "interactive"
	imageTypeMessage    = "message"
	chatTypeP2P         = "p2p"
)

var mentionTagPattern = regexp.MustCompile(`(?s)<at [^>]*>.*?</at>`)

type MessageEvent struct {
	ChatID       string
	MessageID    string
	UserID       string
	Text         string
	ChatType     string
	ThreadID     string
	ParentID     string
	IsAppMention bool
}

type SendOptions struct {
	ReplyToMessageID string
	ThreadID         string
}

type Client struct {
	appID     string
	appSecret string
	api       *lark.Client
}

func NewClient(appID string, appSecret string) *Client {
	appID = strings.TrimSpace(appID)
	appSecret = strings.TrimSpace(appSecret)
	return &Client{
		appID:     appID,
		appSecret: appSecret,
		api:       lark.NewClient(appID, appSecret),
	}
}

func (c *Client) Run(ctx context.Context, handler func(context.Context, MessageEvent) error) error {
	dispatcher := larkdispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(runCtx context.Context, event *larkim.P2MessageReceiveV1) error {
			message, ok, err := parseMessageEvent(event)
			if err != nil || !ok {
				return err
			}
			return handler(runCtx, message)
		})

	wsClient := larkws.NewClient(c.appID, c.appSecret, larkws.WithEventHandler(dispatcher))
	errCh := make(chan error, 1)
	go func() {
		errCh <- wsClient.Start(ctx)
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
}

func (c *Client) SendText(ctx context.Context, chatID string, text string, opts SendOptions) (string, error) {
	content, err := marshalContent(textMessageContent{Text: text})
	if err != nil {
		return "", err
	}
	return c.sendMessage(ctx, chatID, messageTypeText, content, opts)
}

func (c *Client) SendCard(ctx context.Context, chatID string, cardJSON string, opts SendOptions) (string, error) {
	return c.sendMessage(ctx, chatID, messageTypeCard, strings.TrimSpace(cardJSON), opts)
}

func (c *Client) SendImage(ctx context.Context, chatID string, data []byte, opts SendOptions) (string, error) {
	imageKey, err := c.uploadImage(ctx, data)
	if err != nil {
		return "", err
	}
	content, err := marshalContent(imageMessageContent{ImageKey: imageKey})
	if err != nil {
		return "", err
	}
	return c.sendMessage(ctx, chatID, messageTypeImage, content, opts)
}

func (c *Client) uploadImage(ctx context.Context, data []byte) (string, error) {
	req := larkim.NewCreateImageReqBuilder().
		Body(&larkim.CreateImageReqBody{
			ImageType: larkcore.StringPtr(imageTypeMessage),
			Image:     bytes.NewReader(data),
		}).
		Build()
	resp, err := c.api.Im.V1.Image.Create(ctx, req)
	if err != nil {
		return "", err
	}
	if !resp.Success() || resp.Data == nil || resp.Data.ImageKey == nil || strings.TrimSpace(*resp.Data.ImageKey) == "" {
		return "", fmt.Errorf("upload feishu image failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	return strings.TrimSpace(*resp.Data.ImageKey), nil
}

func (c *Client) sendMessage(ctx context.Context, chatID string, msgType string, content string, opts SendOptions) (string, error) {
	if replyTo := strings.TrimSpace(opts.ReplyToMessageID); replyTo != "" {
		replyInThread := strings.TrimSpace(opts.ThreadID) != ""
		resp, err := c.api.Im.V1.Message.Reply(ctx, larkim.NewReplyMessageReqBuilder().
			MessageId(replyTo).
			Body(larkim.NewReplyMessageReqBodyBuilder().
				MsgType(msgType).
				Content(content).
				ReplyInThread(replyInThread).
				Uuid(uuid.NewString()).
				Build()).
			Build())
		if err != nil {
			return "", err
		}
		if !resp.Success() || resp.Data == nil || resp.Data.MessageId == nil || strings.TrimSpace(*resp.Data.MessageId) == "" {
			return "", fmt.Errorf("reply feishu message failed: code=%d msg=%s", resp.Code, resp.Msg)
		}
		return strings.TrimSpace(*resp.Data.MessageId), nil
	}

	resp, err := c.api.Im.V1.Message.Create(ctx, larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(receiveIDTypeChatID).
		Body(&larkim.CreateMessageReqBody{
			ReceiveId: larkcore.StringPtr(strings.TrimSpace(chatID)),
			MsgType:   larkcore.StringPtr(msgType),
			Content:   larkcore.StringPtr(content),
			Uuid:      larkcore.StringPtr(uuid.NewString()),
		}).
		Build())
	if err != nil {
		return "", err
	}
	if !resp.Success() || resp.Data == nil || resp.Data.MessageId == nil || strings.TrimSpace(*resp.Data.MessageId) == "" {
		return "", fmt.Errorf("send feishu message failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	return strings.TrimSpace(*resp.Data.MessageId), nil
}

type textMessageContent struct {
	Text string `json:"text"`
}

type imageMessageContent struct {
	ImageKey string `json:"image_key"`
}

func marshalContent(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func parseMessageEvent(event *larkim.P2MessageReceiveV1) (MessageEvent, bool, error) {
	if event == nil || event.Event == nil || event.Event.Message == nil {
		return MessageEvent{}, false, nil
	}

	message := event.Event.Message
	if strings.TrimSpace(larkcore.StringValue(message.MessageType)) != messageTypeText {
		return MessageEvent{}, false, nil
	}

	content, err := parseTextContent(larkcore.StringValue(message.Content))
	if err != nil {
		return MessageEvent{}, false, err
	}
	isAppMention := len(message.Mentions) > 0
	text := normalizeIncomingText(content.Text)
	if strings.TrimSpace(larkcore.StringValue(message.ChatType)) != chatTypeP2P {
		text = normalizeMentionCommandText(text)
	}
	if text == "" && isAppMention {
		text = "help"
	}
	if text == "" {
		return MessageEvent{}, false, nil
	}

	return MessageEvent{
		ChatID:       strings.TrimSpace(larkcore.StringValue(message.ChatId)),
		MessageID:    strings.TrimSpace(larkcore.StringValue(message.MessageId)),
		UserID:       strings.TrimSpace(eventSenderID(event.Event.Sender)),
		Text:         text,
		ChatType:     strings.TrimSpace(larkcore.StringValue(message.ChatType)),
		ThreadID:     strings.TrimSpace(larkcore.StringValue(message.ThreadId)),
		ParentID:     strings.TrimSpace(larkcore.StringValue(message.ParentId)),
		IsAppMention: isAppMention,
	}, true, nil
}

func parseTextContent(raw string) (textMessageContent, error) {
	body := textMessageContent{}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return textMessageContent{}, fmt.Errorf("decode feishu message content: %w", err)
	}
	return body, nil
}

func normalizeIncomingText(text string) string {
	text = strings.ReplaceAll(text, "\u00a0", " ")
	return strings.TrimSpace(text)
}

func normalizeMentionCommandText(text string) string {
	text = mentionTagPattern.ReplaceAllString(text, " ")
	return strings.TrimSpace(strings.Join(strings.Fields(text), " "))
}

func eventSenderID(sender *larkim.EventSender) string {
	if sender == nil || sender.SenderId == nil {
		return ""
	}
	for _, value := range []string{
		larkcore.StringValue(sender.SenderId.OpenId),
		larkcore.StringValue(sender.SenderId.UserId),
		larkcore.StringValue(sender.SenderId.UnionId),
	} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
