package daemon

import (
	"context"
	"fmt"
	"io"
	"strconv"
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

func (a *discordAdapter) handleInteractionCreate(_ *discordgo.Session, event *discordgo.InteractionCreate) {
	if event == nil || event.Type != discordgo.InteractionApplicationCommand || a.handler == nil {
		return
	}

	interactionID := a.client.StoreInteraction(event.Interaction)
	if interactionID == "" {
		return
	}
	if err := a.client.DeferInteraction(interactionID); err != nil {
		fmt.Fprintf(a.stderr, "discord: defer interaction: %v\n", err)
		return
	}

	msg := IncomingMessage{
		Chat: ChatRef{
			Platform: a.Platform(),
			ChatID:   strings.TrimSpace(event.ChannelID),
		},
		MessageID: event.ID,
		Text:      buildSlashCommandText(event.ApplicationCommandData()),
	}
	threadID := discordConversationID(strings.TrimSpace(event.ChannelID), strings.TrimSpace(event.GuildID) == "")
	msg.ThreadID = threadID
	msg.PendingScope = threadID
	if event.Member != nil && event.Member.User != nil {
		msg.UserID = strings.TrimSpace(event.Member.User.ID)
	} else if event.User != nil {
		msg.UserID = strings.TrimSpace(event.User.ID)
	}

	ctx := withInteractionReplyContext(context.Background(), interactionID)
	if err := a.handler(ctx, msg); err != nil && a.stderr != nil {
		fmt.Fprintf(a.stderr, "discord interaction error: %v\n", err)
	}
}

func (a *discordAdapter) handleMessageCreate(session *discordgo.Session, event *discordgo.MessageCreate) {
	if event == nil || event.Message == nil || a.handler == nil {
		return
	}
	if event.Author == nil || event.Author.Bot {
		return
	}
	if session != nil && session.State != nil && session.State.User != nil && event.Author.ID == session.State.User.ID {
		return
	}

	text, ok := a.normalizeMessageText(strings.TrimSpace(event.Content), strings.TrimSpace(event.GuildID) == "")
	if !ok {
		return
	}

	channelID := strings.TrimSpace(event.ChannelID)
	threadID := discordConversationID(channelID, strings.TrimSpace(event.GuildID) == "")
	msg := IncomingMessage{
		Chat: ChatRef{
			Platform: a.Platform(),
			ChatID:   channelID,
		},
		MessageID:    strings.TrimSpace(event.ID),
		UserID:       strings.TrimSpace(event.Author.ID),
		Text:         text,
		ThreadID:     threadID,
		PendingScope: threadID,
	}
	if err := a.handler(context.Background(), msg); err != nil && a.stderr != nil {
		fmt.Fprintf(a.stderr, "discord message error: %v\n", err)
	}
}

func (a *discordAdapter) normalizeMessageText(text string, isDM bool) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	if prefixed, ok := trimCommandPrefix(text, a.commandPrefix); ok {
		if !strings.HasPrefix(prefixed, "/") {
			prefixed = "/" + prefixed
		}
		return prefixed, true
	}
	if !isDM {
		return "", false
	}
	return text, true
}

func buildSlashCommandText(data discordgo.ApplicationCommandInteractionData) string {
	parts := []string{"/" + strings.TrimSpace(data.Name)}
	appendSlashCommandOptions(&parts, data.Options)
	return strings.TrimSpace(strings.Join(parts, " "))
}

func appendSlashCommandOptions(parts *[]string, options []*discordgo.ApplicationCommandInteractionDataOption) {
	for _, opt := range options {
		if opt == nil {
			continue
		}
		switch opt.Type {
		case discordgo.ApplicationCommandOptionSubCommand, discordgo.ApplicationCommandOptionSubCommandGroup:
			if strings.TrimSpace(opt.Name) != "" {
				*parts = append(*parts, strings.TrimSpace(opt.Name))
			}
			appendSlashCommandOptions(parts, opt.Options)
		default:
			if value := formatOptionValue(opt); value != "" {
				*parts = append(*parts, value)
			}
		}
	}
}

func formatOptionValue(opt *discordgo.ApplicationCommandInteractionDataOption) string {
	switch opt.Type {
	case discordgo.ApplicationCommandOptionString:
		return opt.StringValue()
	case discordgo.ApplicationCommandOptionInteger:
		return strconv.FormatInt(opt.IntValue(), 10)
	case discordgo.ApplicationCommandOptionBoolean:
		return strconv.FormatBool(opt.BoolValue())
	case discordgo.ApplicationCommandOptionNumber:
		return strconv.FormatFloat(opt.FloatValue(), 'f', -1, 64)
	case discordgo.ApplicationCommandOptionUser,
		discordgo.ApplicationCommandOptionChannel,
		discordgo.ApplicationCommandOptionRole,
		discordgo.ApplicationCommandOptionMentionable,
		discordgo.ApplicationCommandOptionAttachment:
		return strings.TrimSpace(opt.StringValue())
	default:
		if opt.Value == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprintf("%v", opt.Value))
	}
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

func discordCommandOptions(spec botCommandSpec) []*discordgo.ApplicationCommandOption {
	switch spec.Command {
	case "select":
		return []*discordgo.ApplicationCommandOption{{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "pane",
			Description: "Pane id such as %5 or default:%5",
			Required:    true,
		}}
	case "unmanage":
		return []*discordgo.ApplicationCommandOption{{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "pane",
			Description: "Managed pane id such as %5 or default:%5",
			Required:    true,
		}}
	case "snapshot":
		return []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "lines",
				Description: "Number of lines to capture",
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "mode",
				Description: "Snapshot output mode",
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "image", Value: "image"},
					{Name: "text", Value: "text"},
				},
			},
		}
	case "send":
		return []*discordgo.ApplicationCommandOption{{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "text",
			Description: "Text to send to the current pane",
			Required:    true,
		}}
	case "keys":
		return []*discordgo.ApplicationCommandOption{{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "keys",
			Description: "Tmux keys such as C-c, Enter, or PageUp",
			Required:    true,
		}}
	case "enter":
		return []*discordgo.ApplicationCommandOption{{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "text",
			Description: "Optional text to send before Enter",
		}}
	case "follow":
		return []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "mode",
				Description: "Enable or disable follow mode",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "on", Value: "on"},
					{Name: "off", Value: "off"},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "interval",
				Description: "Optional interval such as 2s",
			},
		}
	default:
		return nil
	}
}
