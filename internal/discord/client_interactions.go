package discord

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func (c *Client) StoreInteraction(interaction *discordgo.Interaction) string {
	if interaction == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.interactions[interaction.ID] = interaction
	return interaction.ID
}

func (c *Client) DeferInteraction(interactionID string) error {
	interaction, err := c.interactionByID(interactionID)
	if err != nil {
		return err
	}
	return c.session.InteractionRespond(interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
}

func (c *Client) SendInteractionMessage(interactionID string, content string, embed *EmbedData) (string, error) {
	interaction, err := c.interactionByID(interactionID)
	if err != nil {
		return "", err
	}

	edit := &discordgo.WebhookEdit{}
	content = strings.TrimSpace(content)
	edit.Content = &content
	if embed != nil {
		embeds := []*discordgo.MessageEmbed{embedToDiscord(embed)}
		edit.Embeds = &embeds
	}
	msg, err := c.session.InteractionResponseEdit(interaction, edit)
	if err != nil {
		return "", fmt.Errorf("edit discord interaction response: %w", err)
	}
	c.forgetInteraction(interactionID)
	if msg == nil {
		return "", nil
	}
	return msg.ID, nil
}

func (c *Client) SendInteractionImage(interactionID string, fileName string, data []byte, caption string) (string, error) {
	interaction, err := c.interactionByID(interactionID)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("discord image data is required")
	}
	if strings.TrimSpace(fileName) == "" {
		fileName = "snapshot.png"
	}

	edit := &discordgo.WebhookEdit{
		Files: []*discordgo.File{{
			Name:   fileName,
			Reader: newBytesReader(data),
		}},
	}
	if strings.TrimSpace(caption) != "" {
		caption = strings.TrimSpace(caption)
		edit.Content = &caption
	}

	msg, err := c.session.InteractionResponseEdit(interaction, edit)
	if err != nil {
		return "", fmt.Errorf("edit discord interaction image response: %w", err)
	}
	c.forgetInteraction(interactionID)
	if msg == nil {
		return "", nil
	}
	return msg.ID, nil
}

func (c *Client) interactionByID(interactionID string) (*discordgo.Interaction, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	interaction, ok := c.interactions[strings.TrimSpace(interactionID)]
	if !ok || interaction == nil {
		return nil, fmt.Errorf("interaction %s not found", interactionID)
	}
	return interaction, nil
}

func (c *Client) forgetInteraction(interactionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.interactions, strings.TrimSpace(interactionID))
}
