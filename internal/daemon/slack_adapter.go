package daemon

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	slackclient "github.com/hmgle/tmux-connect/internal/slack"
)

type slackAdapter struct {
	client *slackclient.Client
	stderr io.Writer
	store  *Store

	mu            sync.Mutex
	activeThreads map[string]time.Time
	threadTTL     time.Duration
	maxThreads    int
	now           func() time.Time
}

const (
	defaultSlackThreadTTL  = 24 * time.Hour
	defaultSlackMaxThreads = 2048
)

func newSlackAdapter(cfg Config, stderr io.Writer, store *Store) (platformAdapter, error) {
	if strings.TrimSpace(cfg.SlackBotToken) == "" {
		return nil, fmt.Errorf("slack bot token is required")
	}
	if strings.TrimSpace(cfg.SlackAppToken) == "" {
		return nil, fmt.Errorf("slack app token is required")
	}
	return &slackAdapter{
		client:        slackclient.NewClient(cfg.SlackBotToken, cfg.SlackAppToken),
		stderr:        stderr,
		store:         store,
		activeThreads: make(map[string]time.Time),
		threadTTL:     defaultSlackThreadTTL,
		maxThreads:    defaultSlackMaxThreads,
		now:           time.Now,
	}, nil
}

func (a *slackAdapter) Platform() string { return "slack" }

func (a *slackAdapter) SendMessage(ctx context.Context, chat ChatRef, text string, opts SendOptions) (OutboundMessage, error) {
	threadID := strings.TrimSpace(opts.ThreadID)
	if threadID == "" {
		threadID = strings.TrimSpace(opts.ReplyToMessageID)
	}
	ts, err := a.client.PostMessage(ctx, chat.ChatID, text, threadID)
	if err != nil {
		return OutboundMessage{}, err
	}
	if threadID != "" {
		a.rememberThread(chat.ChatID, threadID)
	}
	return OutboundMessage{MessageID: ts}, nil
}

func (a *slackAdapter) SendImage(ctx context.Context, chat ChatRef, fileName string, data []byte, caption string, opts SendOptions) (OutboundMessage, error) {
	threadID := strings.TrimSpace(opts.ThreadID)
	if threadID == "" {
		threadID = strings.TrimSpace(opts.ReplyToMessageID)
	}
	id, err := a.client.UploadImage(ctx, chat.ChatID, threadID, fileName, data, caption)
	if err != nil {
		return OutboundMessage{}, err
	}
	if threadID != "" {
		a.rememberThread(chat.ChatID, threadID)
	}
	return OutboundMessage{MessageID: id}, nil
}

func (a *slackAdapter) DecorateMessage(kind string, text string, opts SendOptions) (string, SendOptions) {
	return decorateSlackMessage(kind, text, opts)
}

func (a *slackAdapter) PromptOptions(message IncomingMessage, _ commandPromptSpec) SendOptions {
	return SendOptions{ThreadID: message.replyThreadID()}
}

func (a *slackAdapter) SnapshotCaption(paneKey string) string {
	return formatSnapshotCaption(paneKey)
}

func (a *slackAdapter) Run(ctx context.Context, handler func(context.Context, IncomingMessage) error) error {
	go func() {
		if err := a.client.Run(ctx); err != nil && ctx.Err() == nil {
			fmt.Fprintf(a.stderr, "slack socket mode error: %v\n", err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-a.client.Events():
			if !ok {
				return nil
			}
			if err := a.handleEvent(ctx, evt, handler); err != nil {
				fmt.Fprintf(a.stderr, "slack event error: %v\n", err)
			}
		}
	}
}

func (a *slackAdapter) RegisterCommands(context.Context, []botCommandSpec) error {
	return nil
}

func (a *slackAdapter) Close() error { return nil }

func (m IncomingMessage) replyThreadID() string {
	if strings.TrimSpace(m.ThreadID) != "" {
		return strings.TrimSpace(m.ThreadID)
	}
	return strings.TrimSpace(m.MessageID)
}
