package daemon

import (
	"context"
	"io"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/hmgle/tmux-connect/internal/discord"
)

type fakeDiscordGatewayClient struct {
	commandPrefix          string
	storedInteractions     []*discordgo.Interaction
	deferredInteractionIDs []string
	interactionMessages    []interactionMessageCall
	interactionImages      []interactionImageCall
	threadMessages         []threadMessageCall
	images                 []threadImageCall
	registered             []discord.CommandSpec
}

type interactionMessageCall struct {
	interactionID string
	content       string
	embed         *discord.EmbedData
}

type interactionImageCall struct {
	interactionID string
	fileName      string
	caption       string
}

type threadMessageCall struct {
	channelID string
	threadID  string
	content   string
	embed     *discord.EmbedData
}

type threadImageCall struct {
	channelID string
	threadID  string
	fileName  string
	caption   string
}

func (f *fakeDiscordGatewayClient) CommandPrefix() string          { return f.commandPrefix }
func (f *fakeDiscordGatewayClient) Open() error                    { return nil }
func (f *fakeDiscordGatewayClient) Close() error                   { return nil }
func (f *fakeDiscordGatewayClient) AddHandler(handler interface{}) {}
func (f *fakeDiscordGatewayClient) RegisterCommands(_ context.Context, commands []discord.CommandSpec) error {
	f.registered = append([]discord.CommandSpec(nil), commands...)
	return nil
}
func (f *fakeDiscordGatewayClient) StoreInteraction(interaction *discordgo.Interaction) string {
	f.storedInteractions = append(f.storedInteractions, interaction)
	if interaction == nil {
		return ""
	}
	return interaction.ID
}
func (f *fakeDiscordGatewayClient) DeferInteraction(interactionID string) error {
	f.deferredInteractionIDs = append(f.deferredInteractionIDs, interactionID)
	return nil
}
func (f *fakeDiscordGatewayClient) SendInteractionMessage(interactionID string, content string, embed *discord.EmbedData) (string, error) {
	f.interactionMessages = append(f.interactionMessages, interactionMessageCall{
		interactionID: interactionID,
		content:       content,
		embed:         embed,
	})
	return "interaction-message", nil
}
func (f *fakeDiscordGatewayClient) SendInteractionImage(interactionID string, fileName string, data []byte, caption string) (string, error) {
	f.interactionImages = append(f.interactionImages, interactionImageCall{
		interactionID: interactionID,
		fileName:      fileName,
		caption:       caption,
	})
	return "interaction-image", nil
}
func (f *fakeDiscordGatewayClient) SendMessageToThread(_ context.Context, channelID string, threadID string, content string, embed *discord.EmbedData) (string, error) {
	f.threadMessages = append(f.threadMessages, threadMessageCall{
		channelID: channelID,
		threadID:  threadID,
		content:   content,
		embed:     embed,
	})
	return "thread-message", nil
}
func (f *fakeDiscordGatewayClient) SendImage(_ context.Context, channelID string, threadID string, fileName string, data []byte, caption string) (string, error) {
	f.images = append(f.images, threadImageCall{
		channelID: channelID,
		threadID:  threadID,
		fileName:  fileName,
		caption:   caption,
	})
	return "thread-image", nil
}

func TestDiscordAdapterSendMessageUsesInteractionResponse(t *testing.T) {
	t.Parallel()

	client := &fakeDiscordGatewayClient{}
	adapter := &discordAdapter{client: client, stderr: io.Discard, commandPrefix: "tmux:"}

	_, err := adapter.SendMessage(context.Background(), ChatRef{Platform: "discord", ChatID: "C123"}, "hello", SendOptions{
		InteractionID: "i-1",
		Embed:         &EmbedData{Title: "Test"},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if len(client.interactionMessages) != 1 {
		t.Fatalf("interactionMessages = %#v, want one call", client.interactionMessages)
	}
	if len(client.threadMessages) != 0 {
		t.Fatalf("threadMessages = %#v, want none", client.threadMessages)
	}
}

func TestDiscordHandleMessageCreateSupportsGuildPrefixAndDMPlainText(t *testing.T) {
	t.Parallel()

	client := &fakeDiscordGatewayClient{commandPrefix: "tmux:"}
	adapter := &discordAdapter{client: client, stderr: io.Discard, commandPrefix: "tmux:"}

	var got []IncomingMessage
	adapter.handler = func(_ context.Context, message IncomingMessage) error {
		got = append(got, message)
		return nil
	}

	adapter.handleMessageCreate(nil, &discordgo.MessageCreate{Message: &discordgo.Message{
		ID:        "m-1",
		GuildID:   "G123",
		ChannelID: "C123",
		Content:   "tmux: panes",
		Author:    &discordgo.User{ID: "U123"},
	}})
	adapter.handleMessageCreate(nil, &discordgo.MessageCreate{Message: &discordgo.Message{
		ID:        "m-2",
		ChannelID: "D123",
		Content:   "status --short",
		Author:    &discordgo.User{ID: "U123"},
	}})
	adapter.handleMessageCreate(nil, &discordgo.MessageCreate{Message: &discordgo.Message{
		ID:        "m-3",
		GuildID:   "G123",
		ChannelID: "C123",
		Content:   "plain text",
		Author:    &discordgo.User{ID: "U123"},
	}})

	if len(got) != 2 {
		t.Fatalf("got %d messages, want 2", len(got))
	}
	if got[0].Text != "/panes" || got[0].ThreadID != "C123" || got[0].PendingScope != "C123" {
		t.Fatalf("guild message = %#v, want prefixed command in channel scope", got[0])
	}
	if got[1].Text != "status --short" || got[1].ThreadID != "" {
		t.Fatalf("dm message = %#v, want plain text with no thread scope", got[1])
	}
}

func TestDiscordHandleInteractionCreateDefersAndPropagatesContext(t *testing.T) {
	t.Parallel()

	client := &fakeDiscordGatewayClient{commandPrefix: "tmux:"}
	adapter := &discordAdapter{client: client, stderr: io.Discard, commandPrefix: "tmux:"}

	var got IncomingMessage
	var interactionID string
	adapter.handler = func(ctx context.Context, message IncomingMessage) error {
		got = message
		interactionID = applyInteractionReplyContext(ctx, SendOptions{}).InteractionID
		return nil
	}

	adapter.handleInteractionCreate(nil, &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			ID:        "i-1",
			Type:      discordgo.InteractionApplicationCommand,
			GuildID:   "G123",
			ChannelID: "C123",
			Member:    &discordgo.Member{User: &discordgo.User{ID: "U123"}},
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "snapshot",
				Options: []*discordgo.ApplicationCommandInteractionDataOption{{
					Type:  discordgo.ApplicationCommandOptionInteger,
					Name:  "lines",
					Value: float64(80),
				}},
			},
		},
	})

	if len(client.deferredInteractionIDs) != 1 || client.deferredInteractionIDs[0] != "i-1" {
		t.Fatalf("deferredInteractionIDs = %#v, want i-1", client.deferredInteractionIDs)
	}
	if interactionID != "i-1" {
		t.Fatalf("interactionID = %q, want i-1", interactionID)
	}
	if got.Text != "/snapshot 80" || got.ThreadID != "C123" || got.PendingScope != "C123" {
		t.Fatalf("incoming message = %#v, want slash command in channel scope", got)
	}
}

func TestDiscordHandleInteractionCreateIncludesStringArguments(t *testing.T) {
	t.Parallel()

	client := &fakeDiscordGatewayClient{commandPrefix: "tmux:"}
	adapter := &discordAdapter{client: client, stderr: io.Discard, commandPrefix: "tmux:"}

	var got IncomingMessage
	adapter.handler = func(_ context.Context, message IncomingMessage) error {
		got = message
		return nil
	}

	adapter.handleInteractionCreate(nil, &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			ID:        "i-2",
			Type:      discordgo.InteractionApplicationCommand,
			GuildID:   "G123",
			ChannelID: "C123",
			Member:    &discordgo.Member{User: &discordgo.User{ID: "U123"}},
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "select",
				Options: []*discordgo.ApplicationCommandInteractionDataOption{{
					Type:  discordgo.ApplicationCommandOptionString,
					Name:  "pane",
					Value: "%9",
				}},
			},
		},
	})

	if got.Text != "/select %9" {
		t.Fatalf("incoming message text = %q, want /select %%9", got.Text)
	}
}

func TestDiscordAdapterRegisterCommandsMapsCommandSpecs(t *testing.T) {
	t.Parallel()

	client := &fakeDiscordGatewayClient{}
	adapter := &discordAdapter{client: client, stderr: io.Discard, commandPrefix: "tmux:"}

	err := adapter.RegisterCommands(context.Background(), []botCommandSpec{
		{Command: "help", Description: "Show command help"},
		{Command: "select", Description: "Select the current pane"},
	})
	if err != nil {
		t.Fatalf("RegisterCommands() error = %v", err)
	}
	if len(client.registered) != 2 || client.registered[0].Name != "help" || client.registered[1].Name != "select" {
		t.Fatalf("registered = %#v, want mapped command specs", client.registered)
	}
	if len(client.registered[0].Options) != 0 {
		t.Fatalf("help options = %#v, want none", client.registered[0].Options)
	}
	if len(client.registered[1].Options) != 1 {
		t.Fatalf("select options = %#v, want one pane option", client.registered[1].Options)
	}
	option := client.registered[1].Options[0]
	if option.Type != discordgo.ApplicationCommandOptionString || option.Name != "pane" || !option.Required {
		t.Fatalf("select option = %#v, want required string pane", option)
	}
}
