package discord

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func (c *Client) SendMessage(ctx context.Context, channelID string, content string, embed *EmbedData) (string, error) {
	params := &discordgo.MessageSend{
		Content: content,
	}
	if embed != nil {
		params.Embeds = []*discordgo.MessageEmbed{embedToDiscord(embed)}
	}
	msg, err := c.session.ChannelMessageSendComplex(channelID, params)
	if err != nil {
		return "", fmt.Errorf("send discord message: %w", err)
	}
	if msg == nil {
		return "", nil
	}
	return msg.ID, nil
}

func (c *Client) SendMessageToThread(ctx context.Context, channelID string, threadID string, content string, embed *EmbedData) (string, error) {
	targetID := strings.TrimSpace(threadID)
	if targetID == "" {
		targetID = strings.TrimSpace(channelID)
	}
	return c.SendMessage(ctx, targetID, content, embed)
}

func (c *Client) SendImage(ctx context.Context, channelID string, threadID string, fileName string, data []byte, caption string) (string, error) {
	targetID := strings.TrimSpace(threadID)
	if targetID == "" {
		targetID = strings.TrimSpace(channelID)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("discord image data is required")
	}
	if strings.TrimSpace(fileName) == "" {
		fileName = "snapshot.png"
	}
	params := &discordgo.MessageSend{
		Files: []*discordgo.File{{
			Name:   fileName,
			Reader: newBytesReader(data),
		}},
	}
	if strings.TrimSpace(caption) != "" {
		params.Content = strings.TrimSpace(caption)
	}
	msg, err := c.session.ChannelMessageSendComplex(targetID, params)
	if err != nil {
		return "", fmt.Errorf("send discord image: %w", err)
	}
	if msg == nil {
		return "", nil
	}
	return msg.ID, nil
}

func embedToDiscord(e *EmbedData) *discordgo.MessageEmbed {
	if e == nil {
		return nil
	}
	embed := &discordgo.MessageEmbed{
		Title:       e.Title,
		Description: e.Description,
		Color:       e.Color,
	}
	for _, field := range e.Fields {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   field.Name,
			Value:  field.Value,
			Inline: field.Inline,
		})
	}
	if strings.TrimSpace(e.Footer) != "" {
		embed.Footer = &discordgo.MessageEmbedFooter{Text: strings.TrimSpace(e.Footer)}
	}
	return embed
}

type bytesReader struct {
	data []byte
	pos  int
}

func newBytesReader(data []byte) *bytesReader {
	return &bytesReader{data: data}
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
