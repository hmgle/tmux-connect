package daemon

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/hmgle/tmux-connect/internal/feishu"
)

type feishuMessageClient interface {
	Run(context.Context, func(context.Context, feishu.MessageEvent) error) error
	SendText(context.Context, string, string, feishu.SendOptions) (string, error)
	SendCard(context.Context, string, string, feishu.SendOptions) (string, error)
	SendImage(context.Context, string, []byte, feishu.SendOptions) (string, error)
}

type feishuAdapter struct {
	client feishuMessageClient
	stderr io.Writer
}

func newFeishuAdapter(cfg Config, stderr io.Writer) (platformAdapter, error) {
	if strings.TrimSpace(cfg.FeishuAppID) == "" {
		return nil, fmt.Errorf("feishu app id is required")
	}
	if strings.TrimSpace(cfg.FeishuAppSecret) == "" {
		return nil, fmt.Errorf("feishu app secret is required")
	}
	return &feishuAdapter{
		client: feishu.NewClient(cfg.FeishuAppID, cfg.FeishuAppSecret),
		stderr: stderr,
	}, nil
}

func (a *feishuAdapter) Platform() string { return "feishu" }

func (a *feishuAdapter) SendMessage(ctx context.Context, chat ChatRef, text string, opts SendOptions) (OutboundMessage, error) {
	feishuOpts := feishu.SendOptions{
		ReplyToMessageID: strings.TrimSpace(opts.ReplyToMessageID),
		ThreadID:         strings.TrimSpace(opts.ThreadID),
	}
	if cardJSON := strings.TrimSpace(cardJSONString(opts.Card)); cardJSON != "" {
		messageID, err := a.client.SendCard(ctx, chat.ChatID, cardJSON, feishuOpts)
		if err != nil {
			return OutboundMessage{}, err
		}
		return OutboundMessage{MessageID: messageID}, nil
	}
	messageID, err := a.client.SendText(ctx, chat.ChatID, text, feishuOpts)
	if err != nil {
		return OutboundMessage{}, err
	}
	return OutboundMessage{MessageID: messageID}, nil
}

func (a *feishuAdapter) SendImage(ctx context.Context, chat ChatRef, _ string, data []byte, _ string, opts SendOptions) (OutboundMessage, error) {
	messageID, err := a.client.SendImage(ctx, chat.ChatID, data, feishu.SendOptions{
		ReplyToMessageID: strings.TrimSpace(opts.ReplyToMessageID),
		ThreadID:         strings.TrimSpace(opts.ThreadID),
	})
	if err != nil {
		return OutboundMessage{}, err
	}
	return OutboundMessage{MessageID: messageID}, nil
}

func (a *feishuAdapter) DecorateMessage(kind string, text string, opts SendOptions) (string, SendOptions) {
	return decorateFeishuMessage(kind, text, opts)
}

func (a *feishuAdapter) PromptOptions(IncomingMessage, commandPromptSpec) SendOptions {
	return SendOptions{}
}

func (a *feishuAdapter) SnapshotCaption(paneKey string) string {
	return formatSnapshotCaption(paneKey)
}

func (a *feishuAdapter) Run(ctx context.Context, handler func(context.Context, IncomingMessage) error) error {
	return a.client.Run(ctx, func(runCtx context.Context, event feishu.MessageEvent) error {
		message := IncomingMessage{
			Chat: ChatRef{
				Platform: a.Platform(),
				ChatID:   strings.TrimSpace(event.ChatID),
			},
			MessageID:    strings.TrimSpace(event.MessageID),
			UserID:       strings.TrimSpace(event.UserID),
			Text:         strings.TrimSpace(event.Text),
			ChatType:     strings.TrimSpace(event.ChatType),
			ThreadID:     strings.TrimSpace(event.ThreadID),
			IsAppMention: event.IsAppMention,
		}
		if message.ThreadID != "" {
			message.PendingScope = message.ThreadID
		}
		if message.Chat.ChatID == "" || message.MessageID == "" || message.Text == "" {
			return nil
		}
		if err := handler(runCtx, message); err != nil {
			if a.stderr != nil {
				fmt.Fprintf(a.stderr, "feishu router error: %v\n", err)
			}
			return err
		}
		return nil
	})
}

func (a *feishuAdapter) RegisterCommands(context.Context, []botCommandSpec) error {
	return nil
}

func (a *feishuAdapter) Close() error {
	return nil
}

func cardJSONString(card any) string {
	switch value := card.(type) {
	case string:
		return value
	case []byte:
		return string(value)
	default:
		return ""
	}
}

var _ platformAdapter = (*feishuAdapter)(nil)
