package daemon

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/hmgle/tmux-connect/internal/tagb"
	"github.com/hmgle/tmux-connect/internal/telegram"
)

type telegramAdapter struct {
	client *telegram.Client
	stderr io.Writer
}

func newTelegramAdapter(cfg Config, stderr io.Writer) *telegramAdapter {
	return &telegramAdapter{
		client: telegram.NewClient(cfg.TelegramToken,
			telegram.WithBaseURL(cfg.APIBaseURL),
			telegram.WithPollTimeout(cfg.PollTimeout),
		),
		stderr: stderr,
	}
}

func (a *telegramAdapter) Platform() string { return "telegram" }

func (a *telegramAdapter) SendMessage(ctx context.Context, chat ChatRef, text string, opts SendOptions) (OutboundMessage, error) {
	chatID, err := strconv.ParseInt(chat.ChatID, 10, 64)
	if err != nil {
		return OutboundMessage{}, fmt.Errorf("telegram chat id %q: %w", chat.ChatID, err)
	}
	sendOpts := telegram.SendOptions{ReplyMarkup: opts.ReplyMarkup}
	if opts.ReplyToMessageID != "" {
		replyID, err := strconv.ParseInt(opts.ReplyToMessageID, 10, 64)
		if err != nil {
			return OutboundMessage{}, fmt.Errorf("telegram reply message id %q: %w", opts.ReplyToMessageID, err)
		}
		sendOpts.ReplyToMessageID = replyID
	}
	if opts.Format == MessageFormatTelegramHTML {
		sendOpts.ParseMode = telegram.ParseModeHTML
	}
	message, err := a.client.SendMessage(ctx, chatID, text, sendOpts)
	if err != nil {
		return OutboundMessage{}, err
	}
	return OutboundMessage{MessageID: strconv.FormatInt(message.MessageID, 10)}, nil
}

func (a *telegramAdapter) SendImage(ctx context.Context, chat ChatRef, fileName string, data []byte, caption string, opts SendOptions) (OutboundMessage, error) {
	chatID, err := strconv.ParseInt(chat.ChatID, 10, 64)
	if err != nil {
		return OutboundMessage{}, fmt.Errorf("telegram chat id %q: %w", chat.ChatID, err)
	}
	sendOpts := telegram.SendOptions{ReplyMarkup: opts.ReplyMarkup}
	if opts.ReplyToMessageID != "" {
		replyID, err := strconv.ParseInt(opts.ReplyToMessageID, 10, 64)
		if err != nil {
			return OutboundMessage{}, fmt.Errorf("telegram reply message id %q: %w", opts.ReplyToMessageID, err)
		}
		sendOpts.ReplyToMessageID = replyID
	}
	if opts.Format == MessageFormatTelegramHTML {
		sendOpts.ParseMode = telegram.ParseModeHTML
	}
	message, err := a.client.SendPhoto(ctx, chatID, fileName, data, caption, sendOpts)
	if err != nil {
		return OutboundMessage{}, err
	}
	return OutboundMessage{MessageID: strconv.FormatInt(message.MessageID, 10)}, nil
}

func (a *telegramAdapter) DecorateMessage(kind string, text string, opts SendOptions) (string, SendOptions) {
	return decorateTelegramMessage(kind, text, opts)
}

func (a *telegramAdapter) PromptOptions(message IncomingMessage, spec commandPromptSpec) SendOptions {
	return SendOptions{
		ReplyToMessageID: message.MessageID,
		ReplyMarkup: telegram.ForceReply{
			ForceReply:            true,
			InputFieldPlaceholder: spec.Placeholder,
		},
	}
}

func (a *telegramAdapter) SnapshotCaption(paneKey string) string {
	return formatSnapshotCaption(paneKey)
}

func (a *telegramAdapter) Run(ctx context.Context, handler func(context.Context, IncomingMessage) error) error {
	offset, err := a.client.DrainPendingUpdates(ctx)
	if err != nil {
		return tagb.TmuxError("drain telegram updates: %v", err)
	}
	for {
		if ctx.Err() != nil {
			return nil
		}
		updates, err := a.client.GetUpdates(ctx, offset, a.client.PollTimeout())
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			fmt.Fprintf(a.stderr, "telegram polling error: %v\n", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
			}
			continue
		}
		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			if update.Message == nil {
				continue
			}
			text := strings.TrimSpace(update.Message.Text)
			if text == "" {
				continue
			}
			if err := handler(ctx, IncomingMessage{
				Chat: ChatRef{
					Platform: a.Platform(),
					ChatID:   strconv.FormatInt(update.Message.Chat.ID, 10),
				},
				MessageID: strconv.FormatInt(update.Message.MessageID, 10),
				Text:      text,
				ChatType:  update.Message.Chat.Type,
			}); err != nil {
				fmt.Fprintf(a.stderr, "router error: %v\n", err)
			}
		}
	}
}

func (a *telegramAdapter) RegisterCommands(ctx context.Context, specs []botCommandSpec) error {
	commands := make([]telegram.BotCommand, 0, len(specs))
	for _, spec := range specs {
		commands = append(commands, telegram.BotCommand{
			Command:     spec.Command,
			Description: spec.Description,
		})
	}
	if err := a.client.SetMyCommands(ctx, commands); err != nil {
		return tagb.TmuxError("configure telegram commands: %v", err)
	}
	return nil
}

func (a *telegramAdapter) Close() error { return nil }
