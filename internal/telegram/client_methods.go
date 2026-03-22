package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"strings"
	"time"
)

func (c *Client) DrainPendingUpdates(ctx context.Context) (int64, error) {
	var offset int64
	for {
		updates, err := c.GetUpdates(ctx, offset, 0)
		if err != nil {
			return offset, err
		}
		if len(updates) == 0 {
			return offset, nil
		}
		offset = updates[len(updates)-1].UpdateID + 1
	}
}

func (c *Client) GetUpdates(ctx context.Context, offset int64, timeout time.Duration) ([]Update, error) {
	if timeout < 0 {
		timeout = 0
	}
	timeoutSeconds := int(timeout / time.Second)
	req := getUpdatesRequest{
		Offset:         offset,
		TimeoutSeconds: timeoutSeconds,
		AllowedUpdates: []string{"message"},
	}
	var resp apiResponse[[]Update]
	if err := c.call(ctx, "getUpdates", req, &resp); err != nil {
		return nil, err
	}
	return resp.Result, nil
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, opts SendOptions) (Message, error) {
	req := sendMessageRequest{
		ChatID: chatID,
		Text:   text,
	}
	if opts.ParseMode != "" {
		req.ParseMode = string(opts.ParseMode)
	}
	if opts.ReplyToMessageID > 0 {
		req.ReplyParameters = &replyParameters{MessageID: opts.ReplyToMessageID}
	}
	if opts.ReplyMarkup != nil {
		req.ReplyMarkup = opts.ReplyMarkup
	}
	var resp apiResponse[Message]
	if err := c.call(ctx, "sendMessage", req, &resp); err != nil {
		return Message{}, err
	}
	return resp.Result, nil
}

func (c *Client) SetMyCommands(ctx context.Context, commands []BotCommand) error {
	req := setMyCommandsRequest{Commands: append([]BotCommand(nil), commands...)}
	var resp apiResponse[bool]
	return c.call(ctx, "setMyCommands", req, &resp)
}

func (c *Client) SendPhoto(ctx context.Context, chatID int64, fileName string, photo []byte, caption string, opts SendOptions) (Message, error) {
	if len(photo) == 0 {
		return Message{}, fmt.Errorf("telegram photo is required")
	}
	if strings.TrimSpace(fileName) == "" {
		fileName = "snapshot.png"
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("chat_id", fmt.Sprintf("%d", chatID)); err != nil {
		return Message{}, fmt.Errorf("write telegram photo chat_id: %w", err)
	}
	if strings.TrimSpace(caption) != "" {
		if err := writer.WriteField("caption", caption); err != nil {
			return Message{}, fmt.Errorf("write telegram photo caption: %w", err)
		}
	}
	if opts.ParseMode != "" {
		if err := writer.WriteField("parse_mode", string(opts.ParseMode)); err != nil {
			return Message{}, fmt.Errorf("write telegram photo parse_mode: %w", err)
		}
	}
	if opts.ReplyToMessageID > 0 {
		replyBody, err := json.Marshal(replyParameters{MessageID: opts.ReplyToMessageID})
		if err != nil {
			return Message{}, fmt.Errorf("marshal telegram photo reply_parameters: %w", err)
		}
		if err := writer.WriteField("reply_parameters", string(replyBody)); err != nil {
			return Message{}, fmt.Errorf("write telegram photo reply_parameters: %w", err)
		}
	}
	part, err := writer.CreateFormFile("photo", fileName)
	if err != nil {
		return Message{}, fmt.Errorf("write telegram photo file header: %w", err)
	}
	if _, err := part.Write(photo); err != nil {
		return Message{}, fmt.Errorf("write telegram photo bytes: %w", err)
	}
	if err := writer.Close(); err != nil {
		return Message{}, fmt.Errorf("close telegram photo body: %w", err)
	}

	var resp apiResponse[Message]
	if err := c.callMultipart(ctx, "sendPhoto", &body, writer.FormDataContentType(), &resp); err != nil {
		return Message{}, err
	}
	return resp.Result, nil
}
