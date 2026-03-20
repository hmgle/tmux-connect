package discord

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
)

const defaultCommandPrefix = "tmux:"

type sessionAPI interface {
	Open() error
	Close() error
	AddHandler(handler interface{}) func()
	Application(appID string) (*discordgo.Application, error)
	ApplicationCommands(appID, guildID string, options ...discordgo.RequestOption) ([]*discordgo.ApplicationCommand, error)
	ApplicationCommandCreate(appID, guildID string, cmd *discordgo.ApplicationCommand, options ...discordgo.RequestOption) (*discordgo.ApplicationCommand, error)
	ApplicationCommandEdit(appID, guildID, cmdID string, cmd *discordgo.ApplicationCommand, options ...discordgo.RequestOption) (*discordgo.ApplicationCommand, error)
	InteractionRespond(interaction *discordgo.Interaction, resp *discordgo.InteractionResponse, options ...discordgo.RequestOption) error
	InteractionResponseEdit(interaction *discordgo.Interaction, newresp *discordgo.WebhookEdit, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error)
}

type Client struct {
	session       sessionAPI
	commandPrefix string
	stderr        io.Writer

	mu            sync.Mutex
	applicationID string
	interactions  map[string]*discordgo.Interaction
}

type Option func(*Client)

type CommandSpec struct {
	Name        string
	Description string
	Options     []*discordgo.ApplicationCommandOption
}

type EmbedData struct {
	Title       string
	Description string
	Color       int
	Fields      []EmbedField
	Footer      string
}

type EmbedField struct {
	Name   string
	Value  string
	Inline bool
}

func WithCommandPrefix(prefix string) Option {
	return func(c *Client) {
		if strings.TrimSpace(prefix) != "" {
			c.commandPrefix = strings.TrimSpace(prefix)
		}
	}
}

func WithStderr(w io.Writer) Option {
	return func(c *Client) {
		if w != nil {
			c.stderr = w
		}
	}
}

func NewClient(token string, opts ...Option) (*Client, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("discord token is required")
	}

	token = strings.TrimPrefix(token, "Bot ")
	token = strings.TrimPrefix(token, "bot ")

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("create discord session: %w", err)
	}
	session.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentsMessageContent

	client := &Client{
		session:       session,
		commandPrefix: defaultCommandPrefix,
		stderr:        io.Discard,
		interactions:  make(map[string]*discordgo.Interaction),
	}
	for _, opt := range opts {
		opt(client)
	}
	return client, nil
}

func (c *Client) CommandPrefix() string {
	return c.commandPrefix
}

func (c *Client) Open() error {
	return c.session.Open()
}

func (c *Client) Close() error {
	if c.session == nil {
		return nil
	}
	return c.session.Close()
}

func (c *Client) AddHandler(handler interface{}) {
	c.session.AddHandler(handler)
}

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
