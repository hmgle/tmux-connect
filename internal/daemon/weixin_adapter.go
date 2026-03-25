//go:build !no_weixin

package daemon

import (
	"context"
	"fmt"
	"io"
	"strings"

	wx "github.com/hmgle/tmux-connect/internal/weixin"
)

type weixinClient interface {
	Run(context.Context, func(context.Context, wx.MessageEvent) error) error
	SendText(context.Context, string, string) (string, error)
	SendImage(context.Context, string, string, []byte, string) (string, error)
	Close() error
}

type weixinAdapter struct {
	client weixinClient
	stderr io.Writer
}

func newWeixinAdapter(cfg Config, stderr io.Writer, store *Store) (platformAdapter, error) {
	client, err := wx.NewClient(wx.ClientConfig{
		Token:      cfg.WeixinToken,
		BaseURL:    cfg.WeixinBaseURL,
		CDNBaseURL: cfg.WeixinCDNBaseURL,
		RouteTag:   cfg.WeixinRouteTag,
		Stderr:     stderr,
		Store:      store,
	})
	if err != nil {
		return nil, err
	}
	return &weixinAdapter{client: client, stderr: stderr}, nil
}

func (a *weixinAdapter) Platform() string { return "weixin" }

func (a *weixinAdapter) SendMessage(ctx context.Context, chat ChatRef, text string, _ SendOptions) (OutboundMessage, error) {
	messageID, err := a.client.SendText(ctx, chat.ChatID, text)
	if err != nil {
		return OutboundMessage{}, err
	}
	return OutboundMessage{MessageID: messageID}, nil
}

func (a *weixinAdapter) SendImage(ctx context.Context, chat ChatRef, fileName string, data []byte, caption string, _ SendOptions) (OutboundMessage, error) {
	messageID, err := a.client.SendImage(ctx, chat.ChatID, fileName, data, caption)
	if err != nil {
		return OutboundMessage{}, err
	}
	return OutboundMessage{MessageID: messageID}, nil
}

func (a *weixinAdapter) DecorateMessage(kind string, text string, opts SendOptions) (string, SendOptions) {
	return decorateCodeBlockMessage(kind, text, opts)
}

func (a *weixinAdapter) ParseMessage(message IncomingMessage) parsedCommand {
	return defaultParseMessage(message, "")
}

func (a *weixinAdapter) PromptOptions(message IncomingMessage, _ commandPromptSpec) SendOptions {
	return SendOptions{ReplyToMessageID: message.MessageID}
}

func (a *weixinAdapter) PromptText(message IncomingMessage, spec commandPromptSpec) string {
	return defaultPromptText(message, spec)
}

func (a *weixinAdapter) NormalizeSnapshotMode(snapshotMode) snapshotMode {
	return snapshotModeText
}

func (a *weixinAdapter) SnapshotCaption(paneKey string) string {
	return formatSnapshotCaption(paneKey)
}

func (a *weixinAdapter) Run(ctx context.Context, handler func(context.Context, IncomingMessage) error) error {
	return a.client.Run(ctx, func(runCtx context.Context, event wx.MessageEvent) error {
		msg := IncomingMessage{
			Chat: ChatRef{
				Platform: a.Platform(),
				ChatID:   strings.TrimSpace(event.ChatID),
			},
			MessageID: strings.TrimSpace(event.MessageID),
			UserID:    strings.TrimSpace(event.SenderID),
			Text:      strings.TrimSpace(event.Text),
			ChatType:  "private",
		}
		if msg.Chat.ChatID == "" || msg.MessageID == "" || msg.Text == "" {
			return nil
		}
		if err := handler(runCtx, msg); err != nil {
			if a.stderr != nil {
				fmt.Fprintf(a.stderr, "weixin router error: %v\n", err)
			}
			return err
		}
		return nil
	})
}

func (a *weixinAdapter) RegisterCommands(context.Context, []botCommandSpec) error {
	return nil
}

func (a *weixinAdapter) Close() error {
	if a.client == nil {
		return nil
	}
	return a.client.Close()
}

var _ platformAdapter = (*weixinAdapter)(nil)
