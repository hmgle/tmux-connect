package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	ChatID           int64  `json:"chat_id"`
	Text             string `json:"text"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
}

type SendOptions struct {
	ReplyToMessageID int64
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
		ChatID:           chatID,
		Text:             text,
		ReplyToMessageID: opts.ReplyToMessageID,
	}
	var resp apiResponse[Message]
	if err := c.call(ctx, "sendMessage", req, &resp); err != nil {
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
	default:
		return nil
	}
	return nil
}
