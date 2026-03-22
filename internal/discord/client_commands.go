package discord

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func (c *Client) RegisterCommands(ctx context.Context, commands []CommandSpec) error {
	appID, err := c.applicationIDFor(ctx)
	if err != nil {
		return err
	}

	existing, err := c.session.ApplicationCommands(appID, "")
	if err != nil {
		return fmt.Errorf("list discord commands: %w", err)
	}

	existingByName := make(map[string]*discordgo.ApplicationCommand, len(existing))
	for _, cmd := range existing {
		existingByName[cmd.Name] = cmd
	}

	for _, spec := range commands {
		cmd := &discordgo.ApplicationCommand{
			Name:        spec.Name,
			Description: spec.Description,
			Options:     spec.Options,
		}
		current := existingByName[spec.Name]
		switch {
		case current == nil:
			if _, err := c.session.ApplicationCommandCreate(appID, "", cmd); err != nil {
				return fmt.Errorf("create discord command %q: %w", spec.Name, err)
			}
		case !sameCommand(current, cmd):
			if _, err := c.session.ApplicationCommandEdit(appID, "", current.ID, cmd); err != nil {
				return fmt.Errorf("update discord command %q: %w", spec.Name, err)
			}
		}
	}

	return nil
}

func (c *Client) applicationIDFor(_ context.Context) (string, error) {
	c.mu.Lock()
	if c.applicationID != "" {
		defer c.mu.Unlock()
		return c.applicationID, nil
	}
	c.mu.Unlock()

	app, err := c.session.Application("@me")
	if err != nil {
		return "", fmt.Errorf("lookup discord application: %w", err)
	}
	if app == nil || strings.TrimSpace(app.ID) == "" {
		return "", fmt.Errorf("lookup discord application: empty application id")
	}

	c.mu.Lock()
	c.applicationID = strings.TrimSpace(app.ID)
	c.mu.Unlock()
	return strings.TrimSpace(app.ID), nil
}

func sameCommand(left *discordgo.ApplicationCommand, right *discordgo.ApplicationCommand) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.Name != right.Name || left.Description != right.Description || len(left.Options) != len(right.Options) {
		return false
	}
	for idx := range left.Options {
		if !sameCommandOption(left.Options[idx], right.Options[idx]) {
			return false
		}
	}
	return true
}

func sameCommandOption(left *discordgo.ApplicationCommandOption, right *discordgo.ApplicationCommandOption) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.Type != right.Type || left.Name != right.Name || left.Description != right.Description || left.Required != right.Required || len(left.Options) != len(right.Options) {
		return false
	}
	for idx := range left.Options {
		if !sameCommandOption(left.Options[idx], right.Options[idx]) {
			return false
		}
	}
	return true
}
