package telegram

import (
	"net/http"
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

type ParseMode string

const (
	ParseModeHTML ParseMode = "HTML"
)

type SendOptions struct {
	ReplyToMessageID int64
	ParseMode        ParseMode
	ReplyMarkup      any
}

type ForceReply struct {
	ForceReply            bool   `json:"force_reply"`
	InputFieldPlaceholder string `json:"input_field_placeholder,omitempty"`
	Selective             bool   `json:"selective,omitempty"`
}

type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
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
	ReplyMarkup     any              `json:"reply_markup,omitempty"`
}

type replyParameters struct {
	MessageID int64 `json:"message_id"`
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
