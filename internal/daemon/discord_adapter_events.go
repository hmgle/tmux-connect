package daemon

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

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
