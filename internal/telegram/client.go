package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.telegram.org"

type Client struct {
	baseURL     string
	token       string
	httpClient  *http.Client
	pollTimeout time.Duration
}

type Option func(*Client)

func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		if strings.TrimSpace(baseURL) != "" {
			c.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
		}
	}
}

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

func WithPollTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout > 0 {
			c.pollTimeout = timeout
		}
	}
}

func NewClient(token string, opts ...Option) *Client {
	client := &Client{
		baseURL: defaultBaseURL,
		token:   strings.TrimSpace(token),
		httpClient: &http.Client{
			Timeout: 35 * time.Second,
		},
		pollTimeout: 20 * time.Second,
	}
	for _, opt := range opts {
		opt(client)
	}
	return client
}

type Chat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
}

type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	Text      string `json:"text"`
	Chat      Chat   `json:"chat"`
	From      *User  `json:"from"`
}

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

type getUpdatesRequest struct {
	Offset         int64    `json:"offset,omitempty"`
	TimeoutSeconds int      `json:"timeout,omitempty"`
	AllowedUpdates []string `json:"allowed_updates,omitempty"`
}

type sendMessageRequest struct {
	ChatID          int64            `json:"chat_id"`
	Text            string           `json:"text"`
	ParseMode       string           `json:"parse_mode,omitempty"`
	ReplyParameters *replyParameters `json:"reply_parameters,omitempty"`
}

type ParseMode string

const (
	ParseModeHTML ParseMode = "HTML"
)

type replyParameters struct {
	MessageID int64 `json:"message_id"`
}

type SendOptions struct {
	ReplyToMessageID int64
	ParseMode        ParseMode
}

type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

type setMyCommandsRequest struct {
	Commands []BotCommand `json:"commands"`
}

type apiResponse[T any] struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	Result      T      `json:"result"`
}

func (c *Client) PollTimeout() time.Duration {
	return c.pollTimeout
}

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

func (c *Client) call(ctx context.Context, method string, payload any, dest any) error {
	if strings.TrimSpace(c.token) == "" {
		return fmt.Errorf("telegram token is required")
	}
	method = strings.TrimSpace(method)
	if method == "" {
		return fmt.Errorf("telegram method is required")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal telegram request: %w", err)
	}

	endpoint, err := url.JoinPath(c.baseURL, "bot"+c.token, method)
	if err != nil {
		return fmt.Errorf("build telegram url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	return c.do(req, method, dest)
}

func (c *Client) callMultipart(ctx context.Context, method string, body *bytes.Buffer, contentType string, dest any) error {
	if strings.TrimSpace(c.token) == "" {
		return fmt.Errorf("telegram token is required")
	}
	method = strings.TrimSpace(method)
	if method == "" {
		return fmt.Errorf("telegram method is required")
	}

	endpoint, err := url.JoinPath(c.baseURL, "bot"+c.token, method)
	if err != nil {
		return fmt.Errorf("build telegram url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body.Bytes()))
	if err != nil {
		return fmt.Errorf("build telegram request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	return c.do(req, method, dest)
}

func (c *Client) do(req *http.Request, method string, dest any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram %s: %w", method, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return fmt.Errorf("read telegram %s response: %w", method, err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("telegram %s: http %d: %s", method, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("decode telegram %s response: %w", method, err)
	}

	switch value := dest.(type) {
	case *apiResponse[[]Update]:
		if !value.OK {
			return fmt.Errorf("telegram %s failed: %s", method, strings.TrimSpace(value.Description))
		}
	case *apiResponse[Message]:
		if !value.OK {
			return fmt.Errorf("telegram %s failed: %s", method, strings.TrimSpace(value.Description))
		}
	case *apiResponse[bool]:
		if !value.OK {
			return fmt.Errorf("telegram %s failed: %s", method, strings.TrimSpace(value.Description))
		}
	default:
		return nil
	}
	return nil
}
