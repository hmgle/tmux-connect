//go:build !no_whatsapp

package daemon

import (
	"context"
	"fmt"
	"io"
	"strings"

	wa "github.com/hmgle/tmux-connect/internal/whatsapp"
)

type whatsappClient interface {
	Run(context.Context, bool, func(context.Context, wa.MessageEvent) error) error
	SendText(context.Context, string, string, string, string) (string, error)
	SendImage(context.Context, string, string, []byte, string, string, string) (string, error)
	Close() error
}

type whatsappAdapter struct {
	client       whatsappClient
	stderr       io.Writer
	autoMarkRead bool
}

func newWhatsAppAdapter(cfg Config, stderr io.Writer) (platformAdapter, error) {
	client, err := wa.NewClient(context.Background(), cfg.WhatsAppSessionDB, cfg.WhatsAppDeviceName, cfg.WhatsAppAllowSelfChat, stderr)
	if err != nil {
		return nil, err
	}
	return &whatsappAdapter{
		client:       client,
		stderr:       stderr,
		autoMarkRead: cfg.WhatsAppAutoMarkRead,
	}, nil
}

func (a *whatsappAdapter) Platform() string { return "whatsapp" }

func (a *whatsappAdapter) SendMessage(ctx context.Context, chat ChatRef, text string, opts SendOptions) (OutboundMessage, error) {
	messageID, err := a.client.SendText(ctx, chat.ChatID, text, opts.ReplyToMessageID, opts.ReplyToSenderID)
	if err != nil {
		return OutboundMessage{}, err
	}
	return OutboundMessage{MessageID: messageID}, nil
}

func (a *whatsappAdapter) SendImage(ctx context.Context, chat ChatRef, fileName string, data []byte, caption string, opts SendOptions) (OutboundMessage, error) {
	messageID, err := a.client.SendImage(ctx, chat.ChatID, fileName, data, caption, opts.ReplyToMessageID, opts.ReplyToSenderID)
	if err != nil {
		return OutboundMessage{}, err
	}
	return OutboundMessage{MessageID: messageID}, nil
}

func (a *whatsappAdapter) DecorateMessage(kind string, text string, opts SendOptions) (string, SendOptions) {
	return decorateWhatsAppMessage(kind, text, opts)
}

func (a *whatsappAdapter) PromptOptions(message IncomingMessage, _ commandPromptSpec) SendOptions {
	return SendOptions{
		ReplyToMessageID: message.MessageID,
		ReplyToSenderID:  message.Chat.ChatID,
	}
}

func (a *whatsappAdapter) SnapshotCaption(paneKey string) string {
	return formatSnapshotCaption(paneKey)
}

func (a *whatsappAdapter) Run(ctx context.Context, handler func(context.Context, IncomingMessage) error) error {
	return a.client.Run(ctx, a.autoMarkRead, func(runCtx context.Context, event wa.MessageEvent) error {
		msg := IncomingMessage{
			Chat: ChatRef{
				Platform: a.Platform(),
				ChatID:   strings.TrimSpace(event.ChatID),
			},
			MessageID:       strings.TrimSpace(event.MessageID),
			UserID:          strings.TrimSpace(event.SenderID),
			IsFromSelf:      event.IsFromMe,
			Text:            strings.TrimSpace(event.Text),
			ChatType:        "private",
			QuotedMessageID: strings.TrimSpace(event.QuotedMessageID),
			QuotedSenderID:  strings.TrimSpace(event.QuotedSenderID),
		}
		if msg.Chat.ChatID == "" || msg.MessageID == "" || msg.Text == "" {
			return nil
		}
		if err := handler(runCtx, msg); err != nil {
			if a.stderr != nil {
				fmt.Fprintf(a.stderr, "whatsapp router error: %v\n", err)
			}
			return err
		}
		return nil
	})
}

func (a *whatsappAdapter) RegisterCommands(context.Context, []botCommandSpec) error {
	return nil
}

func (a *whatsappAdapter) Close() error {
	if a.client == nil {
		return nil
	}
	return a.client.Close()
}

var _ platformAdapter = (*whatsappAdapter)(nil)
