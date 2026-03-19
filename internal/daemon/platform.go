package daemon

import (
	"context"
	"strings"
)

type ChatRef struct {
	Platform string
	ChatID   string
}

func (c ChatRef) Normalized() ChatRef {
	return ChatRef{
		Platform: strings.TrimSpace(strings.ToLower(c.Platform)),
		ChatID:   strings.TrimSpace(c.ChatID),
	}
}

func (c ChatRef) Valid() bool {
	c = c.Normalized()
	return c.Platform != "" && c.ChatID != ""
}

func (c ChatRef) Key() string {
	c = c.Normalized()
	if c.Platform == "" && c.ChatID == "" {
		return ""
	}
	return c.Platform + ":" + c.ChatID
}

type MessageFormat string

const (
	MessageFormatPlain        MessageFormat = ""
	MessageFormatTelegramHTML MessageFormat = "telegram-html"
)

type SendOptions struct {
	ReplyToMessageID string
	ThreadID         string
	ReplyMarkup      any
	Format           MessageFormat
}

type OutboundMessage struct {
	MessageID string
}

type platformAdapter interface {
	Platform() string
	SendMessage(context.Context, ChatRef, string, SendOptions) (OutboundMessage, error)
	SendImage(context.Context, ChatRef, string, []byte, string, SendOptions) (OutboundMessage, error)
	DecorateMessage(kind string, text string, opts SendOptions) (string, SendOptions)
	PromptOptions(message IncomingMessage, spec commandPromptSpec) SendOptions
	SnapshotCaption(paneKey string) string
	Run(context.Context, func(context.Context, IncomingMessage) error) error
	RegisterCommands(context.Context, []botCommandSpec) error
	Close() error
}
