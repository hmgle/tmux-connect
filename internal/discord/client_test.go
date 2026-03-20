package discord

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/bwmarrin/discordgo"
)

type fakeSession struct {
	app                *discordgo.Application
	commands           []*discordgo.ApplicationCommand
	createCalls        []*discordgo.ApplicationCommand
	editCalls          []*discordgo.ApplicationCommand
	editedCommandIDs   []string
	lastInteraction    *discordgo.Interaction
	lastEdit           *discordgo.WebhookEdit
	lastChannelID      string
	lastMessage        *discordgo.MessageSend
	applicationLookups int
}

func (f *fakeSession) Open() error  { return nil }
func (f *fakeSession) Close() error { return nil }
func (f *fakeSession) AddHandler(handler interface{}) func() {
	return func() {}
}
func (f *fakeSession) Application(appID string) (*discordgo.Application, error) {
	f.applicationLookups++
	if f.app == nil {
		return nil, errors.New("missing app")
	}
	return f.app, nil
}
func (f *fakeSession) ApplicationCommands(appID, guildID string, options ...discordgo.RequestOption) ([]*discordgo.ApplicationCommand, error) {
	return append([]*discordgo.ApplicationCommand(nil), f.commands...), nil
}
func (f *fakeSession) ApplicationCommandCreate(appID, guildID string, cmd *discordgo.ApplicationCommand, options ...discordgo.RequestOption) (*discordgo.ApplicationCommand, error) {
	f.createCalls = append(f.createCalls, cmd)
	return cmd, nil
}
func (f *fakeSession) ApplicationCommandEdit(appID, guildID, cmdID string, cmd *discordgo.ApplicationCommand, options ...discordgo.RequestOption) (*discordgo.ApplicationCommand, error) {
	f.editedCommandIDs = append(f.editedCommandIDs, cmdID)
	f.editCalls = append(f.editCalls, cmd)
	return cmd, nil
}
func (f *fakeSession) InteractionRespond(interaction *discordgo.Interaction, resp *discordgo.InteractionResponse, options ...discordgo.RequestOption) error {
	f.lastInteraction = interaction
	return nil
}
func (f *fakeSession) InteractionResponseEdit(interaction *discordgo.Interaction, newresp *discordgo.WebhookEdit, options ...discordgo.RequestOption) (*discordgo.Message, error) {
	f.lastInteraction = interaction
	f.lastEdit = newresp
	return &discordgo.Message{ID: "edited-message"}, nil
}
func (f *fakeSession) ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error) {
	f.lastChannelID = channelID
	f.lastMessage = data
	return &discordgo.Message{ID: "sent-message"}, nil
}

func newTestClient(session sessionAPI) *Client {
	return &Client{
		session:       session,
		commandPrefix: defaultCommandPrefix,
		stderr:        io.Discard,
		interactions:  make(map[string]*discordgo.Interaction),
	}
}

func TestNewClientRequiresToken(t *testing.T) {
	t.Parallel()

	if _, err := NewClient(""); err == nil {
		t.Fatal("NewClient() error = nil, want error")
	}
}

func TestNewClientAcceptsTokenWithBotPrefix(t *testing.T) {
	t.Parallel()

	client, err := NewClient("Bot test-token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
}

func TestRegisterCommandsLooksUpApplicationAndReconcilesCommands(t *testing.T) {
	t.Parallel()

	session := &fakeSession{
		app: &discordgo.Application{ID: "app-1"},
		commands: []*discordgo.ApplicationCommand{
			{ID: "cmd-1", Name: "panes", Description: "Old description"},
			{ID: "cmd-2", Name: "help", Description: "Show command help"},
		},
	}
	client := newTestClient(session)

	err := client.RegisterCommands(context.Background(), []CommandSpec{
		{Name: "panes", Description: "List managed panes"},
		{Name: "help", Description: "Show command help"},
		{Name: "select", Description: "Select the current pane"},
	})
	if err != nil {
		t.Fatalf("RegisterCommands() error = %v", err)
	}
	if session.applicationLookups != 1 {
		t.Fatalf("applicationLookups = %d, want 1", session.applicationLookups)
	}
	if len(session.editCalls) != 1 || session.editCalls[0].Name != "panes" {
		t.Fatalf("editCalls = %#v, want one panes edit", session.editCalls)
	}
	if len(session.createCalls) != 1 || session.createCalls[0].Name != "select" {
		t.Fatalf("createCalls = %#v, want one select create", session.createCalls)
	}
}

func TestSendInteractionMessageEditsOriginalResponse(t *testing.T) {
	t.Parallel()

	session := &fakeSession{}
	client := newTestClient(session)
	client.StoreInteraction(&discordgo.Interaction{ID: "i-1", Token: "token", AppID: "app-1"})

	msgID, err := client.SendInteractionMessage("i-1", "hello", &EmbedData{Title: "Test"})
	if err != nil {
		t.Fatalf("SendInteractionMessage() error = %v", err)
	}
	if msgID != "edited-message" {
		t.Fatalf("messageID = %q, want edited-message", msgID)
	}
	if session.lastEdit == nil || session.lastEdit.Content == nil || *session.lastEdit.Content != "hello" {
		t.Fatalf("lastEdit = %#v, want content hello", session.lastEdit)
	}
	if session.lastEdit.Embeds == nil || len(*session.lastEdit.Embeds) != 1 {
		t.Fatalf("lastEdit embeds = %#v, want one embed", session.lastEdit)
	}
	if _, err := client.SendInteractionMessage("i-1", "again", nil); err == nil {
		t.Fatal("SendInteractionMessage() second call error = nil, want missing interaction")
	}
}

func TestSendInteractionImageEditsOriginalResponseWithFile(t *testing.T) {
	t.Parallel()

	session := &fakeSession{}
	client := newTestClient(session)
	client.StoreInteraction(&discordgo.Interaction{ID: "i-1", Token: "token", AppID: "app-1"})

	_, err := client.SendInteractionImage("i-1", "snapshot.png", []byte("png"), "pane snapshot")
	if err != nil {
		t.Fatalf("SendInteractionImage() error = %v", err)
	}
	if session.lastEdit == nil || len(session.lastEdit.Files) != 1 {
		t.Fatalf("lastEdit = %#v, want one file", session.lastEdit)
	}
	if session.lastEdit.Content == nil || *session.lastEdit.Content != "pane snapshot" {
		t.Fatalf("lastEdit content = %#v, want pane snapshot", session.lastEdit.Content)
	}
}
