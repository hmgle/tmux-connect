package discord

import (
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
