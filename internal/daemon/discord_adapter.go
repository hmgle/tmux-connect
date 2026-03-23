//go:build !no_discord

package daemon

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/hmgle/tmux-connect/internal/discord"
	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

type discordGatewayClient interface {
	CommandPrefix() string
	Open() error
	Close() error
	AddHandler(handler interface{})
	RegisterCommands(context.Context, []discord.CommandSpec) error
	StoreInteraction(*discordgo.Interaction) string
	DeferInteraction(string) error
	SendInteractionMessage(string, string, *discord.EmbedData) (string, error)
	SendInteractionImage(string, string, []byte, string) (string, error)
	SendMessageToThread(context.Context, string, string, string, *discord.EmbedData) (string, error)
	SendImage(context.Context, string, string, string, []byte, string) (string, error)
}

type discordAdapter struct {
	client        discordGatewayClient
	stderr        io.Writer
	commandPrefix string
	handler       func(context.Context, IncomingMessage) error
}

func newDiscordAdapter(cfg Config, stderr io.Writer) (*discordAdapter, error) {
	token := strings.TrimSpace(cfg.DiscordToken)
	if token == "" {
		return nil, fmt.Errorf("discord token is required")
	}

	prefix := strings.TrimSpace(cfg.DiscordCommandPrefix)
	if prefix == "" {
		prefix = defaultDiscordCommandPrefix
	}

	client, err := discord.NewClient(token,
		discord.WithCommandPrefix(prefix),
		discord.WithStderr(stderr),
	)
	if err != nil {
		return nil, err
	}

	return &discordAdapter{
		client:        client,
		stderr:        stderr,
		commandPrefix: prefix,
	}, nil
}

func (a *discordAdapter) Platform() string { return "discord" }

func (a *discordAdapter) SendMessage(ctx context.Context, chat ChatRef, text string, opts SendOptions) (OutboundMessage, error) {
	var embed *discord.EmbedData
	if opts.Embed != nil {
		embed = daemonEmbedToDiscord(opts.Embed)
	}
	if interactionID := strings.TrimSpace(opts.InteractionID); interactionID != "" {
		msgID, err := a.client.SendInteractionMessage(interactionID, text, embed)
		if err != nil {
			return OutboundMessage{}, err
		}
		return OutboundMessage{MessageID: msgID}, nil
	}

	threadID := strings.TrimSpace(opts.ThreadID)
	if threadID == "" {
		threadID = strings.TrimSpace(opts.ReplyToMessageID)
	}
	msgID, err := a.client.SendMessageToThread(ctx, chat.ChatID, threadID, text, embed)
	if err != nil {
		return OutboundMessage{}, err
	}
	return OutboundMessage{MessageID: msgID}, nil
}

func (a *discordAdapter) SendImage(ctx context.Context, chat ChatRef, fileName string, data []byte, caption string, opts SendOptions) (OutboundMessage, error) {
	if interactionID := strings.TrimSpace(opts.InteractionID); interactionID != "" {
		msgID, err := a.client.SendInteractionImage(interactionID, fileName, data, caption)
		if err != nil {
			return OutboundMessage{}, err
		}
		return OutboundMessage{MessageID: msgID}, nil
	}

	threadID := strings.TrimSpace(opts.ThreadID)
	if threadID == "" {
		threadID = strings.TrimSpace(opts.ReplyToMessageID)
	}
	msgID, err := a.client.SendImage(ctx, chat.ChatID, threadID, fileName, data, caption)
	if err != nil {
		return OutboundMessage{}, err
	}
	return OutboundMessage{MessageID: msgID}, nil
}

func (a *discordAdapter) DecorateMessage(kind string, text string, opts SendOptions) (string, SendOptions) {
	return decorateDiscordMessage(kind, text, opts)
}

func (a *discordAdapter) PromptOptions(message IncomingMessage, _ commandPromptSpec) SendOptions {
	return SendOptions{ThreadID: message.ThreadID}
}

func (a *discordAdapter) SnapshotCaption(paneKey string) string {
	return formatSnapshotCaption(paneKey)
}

func (a *discordAdapter) Run(ctx context.Context, handler func(context.Context, IncomingMessage) error) error {
	a.handler = handler
	a.client.AddHandler(a.handleInteractionCreate)
	a.client.AddHandler(a.handleMessageCreate)
	if err := a.client.Open(); err != nil {
		return tmuxconn.TmuxError("open discord gateway: %v", err)
	}
	<-ctx.Done()
	return nil
}

func (a *discordAdapter) RegisterCommands(ctx context.Context, specs []botCommandSpec) error {
	commands := make([]discord.CommandSpec, 0, len(specs))
	for _, spec := range specs {
		commands = append(commands, discord.CommandSpec{
			Name:        spec.Command,
			Description: spec.Description,
			Options:     discordCommandOptions(spec),
		})
	}
	return a.client.RegisterCommands(ctx, commands)
}

func (a *discordAdapter) Close() error {
	return a.client.Close()
}

func daemonEmbedToDiscord(embed *EmbedData) *discord.EmbedData {
	if embed == nil {
		return nil
	}
	out := &discord.EmbedData{
		Title:       embed.Title,
		Description: embed.Description,
		Color:       embed.Color,
		Footer:      embed.Footer,
	}
	for _, field := range embed.Fields {
		out.Fields = append(out.Fields, discord.EmbedField{
			Name:   field.Name,
			Value:  field.Value,
			Inline: field.Inline,
		})
	}
	return out
}

func discordConversationID(channelID string, isDM bool) string {
	if isDM {
		return ""
	}
	return strings.TrimSpace(channelID)
}
