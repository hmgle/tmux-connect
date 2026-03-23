package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

type BotIdentity struct {
	OpenID  string
	UserID  string
	UnionID string
}

type Client struct {
	appID         string
	appSecret     string
	api           *lark.Client
	logger        larkcore.Logger
	botMentionIDs map[string]struct{}
}

func NewClient(appID string, appSecret string, identity BotIdentity, logWriter io.Writer) *Client {
	appID = strings.TrimSpace(appID)
	appSecret = strings.TrimSpace(appSecret)
	logger := newSDKLogger(logWriter)
	return &Client{
		appID:         appID,
		appSecret:     appSecret,
		api:           lark.NewClient(appID, appSecret, lark.WithLogger(logger)),
		logger:        logger,
		botMentionIDs: botIdentitySet(identity),
	}
}

func (c *Client) Run(ctx context.Context, handler func(context.Context, MessageEvent) error) error {
	dispatcher := larkdispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(runCtx context.Context, event *larkim.P2MessageReceiveV1) error {
			message, ok, err := parseMessageEvent(event, c.botMentionIDs)
			if err != nil || !ok {
				return err
			}
			return handler(runCtx, message)
		})

	wsClient := larkws.NewClient(c.appID, c.appSecret,
		larkws.WithEventHandler(dispatcher),
		larkws.WithLogger(c.logger),
	)
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
		return wrapFeishuError("start websocket client", err)
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
		return "", wrapFeishuError("upload image", err)
	}
	if !resp.Success() || resp.Data == nil || resp.Data.ImageKey == nil || strings.TrimSpace(*resp.Data.ImageKey) == "" {
		return "", feishuAPIError("upload image", resp.Code, resp.Msg)
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
			return "", wrapFeishuError("reply message", err)
		}
		if !resp.Success() || resp.Data == nil || resp.Data.MessageId == nil || strings.TrimSpace(*resp.Data.MessageId) == "" {
			return "", feishuAPIError("reply message", resp.Code, resp.Msg)
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
		return "", wrapFeishuError("send message", err)
	}
	if !resp.Success() || resp.Data == nil || resp.Data.MessageId == nil || strings.TrimSpace(*resp.Data.MessageId) == "" {
		return "", feishuAPIError("send message", resp.Code, resp.Msg)
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

func parseMessageEvent(event *larkim.P2MessageReceiveV1, botMentionIDs map[string]struct{}) (MessageEvent, bool, error) {
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
	isAppMention := hasBotMention(message.Mentions, botMentionIDs)
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

func botIdentitySet(identity BotIdentity) map[string]struct{} {
	ids := make(map[string]struct{}, 3)
	for _, value := range []string{
		strings.TrimSpace(identity.OpenID),
		strings.TrimSpace(identity.UserID),
		strings.TrimSpace(identity.UnionID),
	} {
		if value != "" {
			ids[value] = struct{}{}
		}
	}
	return ids
}

func hasBotMention(mentions []*larkim.MentionEvent, botMentionIDs map[string]struct{}) bool {
	if len(mentions) == 0 {
		return false
	}
	if len(botMentionIDs) == 0 {
		// Backward-compatible fallback until the bot identity is configured.
		return true
	}
	for _, mention := range mentions {
		if mentionMatchesBotIdentity(mention, botMentionIDs) {
			return true
		}
	}
	return false
}

func mentionMatchesBotIdentity(mention *larkim.MentionEvent, botMentionIDs map[string]struct{}) bool {
	if mention == nil || mention.Id == nil {
		return false
	}
	for _, value := range []string{
		larkcore.StringValue(mention.Id.OpenId),
		larkcore.StringValue(mention.Id.UserId),
		larkcore.StringValue(mention.Id.UnionId),
	} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := botMentionIDs[value]; ok {
			return true
		}
	}
	return false
}

func feishuAPIError(action string, code int, msg string) error {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		msg = "unknown error"
	}
	hint := feishuErrorHint(msg)
	if hint == "" {
		return fmt.Errorf("feishu %s failed: code=%d msg=%s", strings.TrimSpace(action), code, msg)
	}
	return fmt.Errorf("feishu %s failed: code=%d msg=%s; %s", strings.TrimSpace(action), code, msg, hint)
}

func wrapFeishuError(action string, err error) error {
	if err == nil {
		return nil
	}
	hint := feishuErrorHint(err.Error())
	if hint == "" {
		return fmt.Errorf("feishu %s: %w", strings.TrimSpace(action), err)
	}
	return fmt.Errorf("feishu %s: %w; %s", strings.TrimSpace(action), err, hint)
}

func feishuErrorHint(text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	switch {
	case lower == "":
		return ""
	case strings.Contains(lower, "app_access_token"),
		strings.Contains(lower, "tenant_access_token"),
		strings.Contains(lower, "app_secret"),
		strings.Contains(lower, "app_id"),
		strings.Contains(lower, "unauthorized"),
		strings.Contains(lower, "permission"):
		return "check TMUXCONN_FEISHU_APP_ID/TMUXCONN_FEISHU_APP_SECRET and the Feishu app permissions"
	default:
		return ""
	}
}
