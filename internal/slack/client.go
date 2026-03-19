package slack

import (
	"bytes"
	"context"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

type Client struct {
	api    *slack.Client
	socket *socketmode.Client
}

func NewClient(botToken string, appToken string) *Client {
	api := slack.New(botToken, slack.OptionAppLevelToken(appToken))
	return &Client{
		api:    api,
		socket: socketmode.New(api),
	}
}

func (c *Client) Events() <-chan socketmode.Event {
	return c.socket.Events
}

func (c *Client) Ack(req socketmode.Request) {
	c.socket.Ack(req)
}

func (c *Client) Run(ctx context.Context) error {
	return c.socket.RunContext(ctx)
}

func (c *Client) PostMessage(ctx context.Context, channel string, text string, threadID string) (string, error) {
	opts := []slack.MsgOption{slack.MsgOptionText(text, false)}
	if threadID != "" {
		opts = append(opts, slack.MsgOptionTS(threadID))
	}
	_, ts, err := c.api.PostMessageContext(ctx, channel, opts...)
	if err != nil {
		return "", err
	}
	return ts, nil
}

func (c *Client) UploadImage(ctx context.Context, channel string, threadID string, fileName string, data []byte, caption string) (string, error) {
	file, err := c.api.UploadFileContext(ctx, slack.FileUploadParameters{
		Channels:        []string{channel},
		Filename:        fileName,
		Filetype:        "png",
		Title:           caption,
		InitialComment:  caption,
		Reader:          bytes.NewReader(data),
		ThreadTimestamp: threadID,
	})
	if err != nil {
		return "", err
	}
	return file.ID, nil
}
