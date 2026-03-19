package slack

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	slackapi "github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

type Client struct {
	api    *slackapi.Client
	socket *socketmode.Client
}

func NewClient(botToken string, appToken string, options ...slackapi.Option) *Client {
	options = append([]slackapi.Option{slackapi.OptionAppLevelToken(appToken)}, options...)
	api := slackapi.New(botToken, options...)
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
	opts := []slackapi.MsgOption{slackapi.MsgOptionText(text, false)}
	if threadID != "" {
		opts = append(opts, slackapi.MsgOptionTS(threadID))
	}
	_, ts, err := c.api.PostMessageContext(ctx, channel, opts...)
	if err != nil {
		return "", err
	}
	return ts, nil
}

func (c *Client) UploadImage(ctx context.Context, channel string, threadID string, fileName string, data []byte, caption string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("slack image upload requires data")
	}
	if strings.TrimSpace(fileName) == "" {
		fileName = "snapshot.png"
	}
	file, err := c.api.UploadFileV2Context(ctx, slackapi.UploadFileV2Parameters{
		Filename:        fileName,
		FileSize:        len(data),
		Title:           caption,
		InitialComment:  caption,
		Reader:          bytes.NewReader(data),
		Channel:         channel,
		ThreadTimestamp: threadID,
	})
	if err != nil {
		return "", annotateUploadImageError(err)
	}
	return file.ID, nil
}

func annotateUploadImageError(err error) error {
	if err == nil {
		return nil
	}

	var slackErr slackapi.SlackErrorResponse
	if errors.As(err, &slackErr) {
		switch strings.TrimSpace(slackErr.Err) {
		case "missing_scope", "not_allowed_token_type", "no_permission":
			return fmt.Errorf("%w (check Slack bot scope files:write and reinstall the app after updating scopes)", err)
		}
	}
	return err
}
