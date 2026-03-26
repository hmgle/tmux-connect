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

type EmbedField struct {
	Name   string
	Value  string
	Inline bool
}

type EmbedData struct {
	Title       string
	Description string
	Color       int
	Fields      []EmbedField
	Footer      string
}

type SendOptions struct {
	ReplyToMessageID string
	ReplyToSenderID  string
	ThreadID         string
	ReplyMarkup      any
	Card             any
	Format           MessageFormat
	Embed            *EmbedData
	InteractionID    string
}

type OutboundMessage struct {
	MessageID string
}

type parsedCommand struct {
	Command string
	Args    string
	Ignore  bool
}

type platformMessageSender interface {
	SendMessage(context.Context, ChatRef, string, SendOptions) (OutboundMessage, error)
	SendImage(context.Context, ChatRef, string, []byte, string, SendOptions) (OutboundMessage, error)
}

type platformMessageFormatter interface {
	DecorateMessage(kind string, text string, opts SendOptions) (string, SendOptions)
	PromptOptions(message IncomingMessage, spec commandPromptSpec) SendOptions
	PromptText(message IncomingMessage, spec commandPromptSpec) string
	NormalizeSnapshotMode(mode snapshotMode) snapshotMode
	SnapshotCaption(paneKey string) string
	HelpText() string
}

type platformRunner interface {
	Platform() string
	ParseMessage(message IncomingMessage) parsedCommand
	Run(context.Context, func(context.Context, IncomingMessage) error) error
	RegisterCommands(context.Context, []botCommandSpec) error
	Close() error
}

type platformAdapter interface {
	platformMessageSender
	platformMessageFormatter
	platformRunner
}
