package daemon

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hmgle/tmux-connect/internal/telegram"
	"github.com/hmgle/tmux-connect/internal/tmux"
	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

type fakeMessenger struct {
	mu              sync.Mutex
	messages        []sentMessage
	platform        string
	commandPrefix   string
	sendMessageErrs []error
	sendImageErrs   []error
}

type sentMessage struct {
	Text              string
	Card              any
	Caption           string
	FileName          string
	Photo             []byte
	Embed             *EmbedData
	ParseMode         telegram.ParseMode
	ReplyToMessageID  int64
	ReplyToMessageRef string
	ReplyMarkup       any
	ThreadID          string
	InteractionID     string
}

func (m *fakeMessenger) Platform() string {
	if strings.TrimSpace(m.platform) == "" {
		return "telegram"
	}
	return m.platform
}

func (m *fakeMessenger) effectiveCommandPrefix() string {
	if strings.TrimSpace(m.commandPrefix) != "" {
		return strings.TrimSpace(m.commandPrefix)
	}
	switch m.Platform() {
	case "slack":
		return defaultSlackCommandPrefix
	case "discord":
		return defaultDiscordCommandPrefix
	default:
		return ""
	}
}

func (m *fakeMessenger) SendMessage(_ context.Context, _ ChatRef, text string, opts SendOptions) (OutboundMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sendMessageErrs) > 0 {
		err := m.sendMessageErrs[0]
		m.sendMessageErrs = m.sendMessageErrs[1:]
		return OutboundMessage{}, err
	}
	parseMode := telegram.ParseMode("")
	if opts.Format == MessageFormatTelegramHTML {
		parseMode = telegram.ParseModeHTML
	}
	replyTo := int64(0)
	if opts.ReplyToMessageID != "" {
		parsed, _ := strconv.ParseInt(opts.ReplyToMessageID, 10, 64)
		replyTo = parsed
	}
	m.messages = append(m.messages, sentMessage{
		Text:              text,
		Card:              opts.Card,
		Embed:             opts.Embed,
		ParseMode:         parseMode,
		ReplyToMessageID:  replyTo,
		ReplyToMessageRef: opts.ReplyToMessageID,
		ReplyMarkup:       opts.ReplyMarkup,
		ThreadID:          opts.ThreadID,
		InteractionID:     opts.InteractionID,
	})
	return OutboundMessage{MessageID: strconv.Itoa(len(m.messages))}, nil
}

func (m *fakeMessenger) SendImage(_ context.Context, _ ChatRef, fileName string, photo []byte, caption string, opts SendOptions) (OutboundMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sendImageErrs) > 0 {
		err := m.sendImageErrs[0]
		m.sendImageErrs = m.sendImageErrs[1:]
		return OutboundMessage{}, err
	}
	parseMode := telegram.ParseMode("")
	if opts.Format == MessageFormatTelegramHTML {
		parseMode = telegram.ParseModeHTML
	}
	replyTo := int64(0)
	if opts.ReplyToMessageID != "" {
		parsed, _ := strconv.ParseInt(opts.ReplyToMessageID, 10, 64)
		replyTo = parsed
	}
	m.messages = append(m.messages, sentMessage{
		Caption:           caption,
		FileName:          fileName,
		Photo:             append([]byte(nil), photo...),
		Embed:             opts.Embed,
		ParseMode:         parseMode,
		ReplyToMessageID:  replyTo,
		ReplyToMessageRef: opts.ReplyToMessageID,
		ThreadID:          opts.ThreadID,
		InteractionID:     opts.InteractionID,
	})
	return OutboundMessage{MessageID: strconv.Itoa(len(m.messages))}, nil
}

func (m *fakeMessenger) DecorateMessage(kind string, text string, opts SendOptions) (string, SendOptions) {
	if m.Platform() == "slack" {
		return decorateSlackMessage(kind, text, opts)
	}
	if m.Platform() == "feishu" {
		return decorateFeishuMessage(kind, text, opts)
	}
	if m.Platform() == "discord" {
		return decorateDiscordMessage(kind, text, opts)
	}
	if m.Platform() == "whatsapp" {
		return decorateWhatsAppMessage(kind, text, opts)
	}
	if m.Platform() == "weixin" {
		return decorateCodeBlockMessage(kind, text, opts)
	}
	return decorateTelegramMessage(kind, text, opts)
}

func (m *fakeMessenger) ParseMessage(message IncomingMessage) parsedCommand {
	switch m.Platform() {
	case "slack":
		return defaultParseMessage(message, m.effectiveCommandPrefix())
	case "discord":
		return defaultParseMessage(message, m.effectiveCommandPrefix())
	case "feishu":
		if !isFeishuDirectMessage(message) && !message.IsAppMention {
			return parsedCommand{Ignore: true}
		}
		return defaultParseMessage(message, "")
	default:
		return defaultParseMessage(message, "")
	}
}

func (m *fakeMessenger) PromptOptions(message IncomingMessage, spec commandPromptSpec) SendOptions {
	if m.Platform() == "slack" {
		return SendOptions{ThreadID: message.replyThreadID()}
	}
	if m.Platform() == "whatsapp" {
		return SendOptions{ReplyToMessageID: message.MessageID, ReplyToSenderID: message.Chat.ChatID}
	}
	if m.Platform() == "feishu" {
		return feishuReplyOptions(message)
	}
	return SendOptions{
		ReplyToMessageID: message.MessageID,
		ReplyMarkup: telegram.ForceReply{
			ForceReply:            true,
			InputFieldPlaceholder: spec.Placeholder,
		},
	}
}

func (m *fakeMessenger) PromptText(message IncomingMessage, spec commandPromptSpec) string {
	if m.Platform() == "discord" && strings.TrimSpace(message.ThreadID) != "" {
		return spec.Message + "\n\nIn Discord channels, reply with " + strconv.Quote(m.effectiveCommandPrefix()+" <value>") + "."
	}
	return spec.Message
}

func (m *fakeMessenger) NormalizeSnapshotMode(mode snapshotMode) snapshotMode {
	if m.Platform() == "weixin" {
		return snapshotModeText
	}
	return mode
}

func (m *fakeMessenger) SnapshotCaption(paneKey string) string {
	return formatSnapshotCaption(paneKey)
}

func (m *fakeMessenger) HelpText() string {
	switch m.Platform() {
	case "slack":
		return platformHelpText("slack", m.effectiveCommandPrefix())
	case "discord":
		return platformHelpText("discord", m.effectiveCommandPrefix())
	default:
		return platformHelpText(m.Platform(), "")
	}
}

func (m *fakeMessenger) Run(context.Context, func(context.Context, IncomingMessage) error) error {
	return nil
}

func (m *fakeMessenger) RegisterCommands(context.Context, []botCommandSpec) error {
	return nil
}

func (m *fakeMessenger) Close() error {
	return nil
}

func (m *fakeMessenger) snapshot() []sentMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]sentMessage, len(m.messages))
	copy(out, m.messages)
	return out
}

type fakePaneService struct {
	records          map[string]tmuxconn.PaneRecord
	sub              *tmux.Subscription
	snapshotText     string
	snapshotRich     string
	snapshotSequence []snapshotResult
	snapshotCalls    int
	sendErr          error
	initialOutput    string
	sendCalls        []sendCall
	keyCalls         []keyCall
	listCalls        int
	attachCalls      int
	detachCalls      int
}

type sendCall struct {
	paneKey   string
	text      string
	sendEnter bool
}

type snapshotResult struct {
	body string
	err  error
}

type keyCall struct {
	paneKey string
	keys    []string
}

func newFakePaneService() *fakePaneService {
	record := tmuxconn.PaneRecord{
		Info: tmux.PaneInfo{
			Target:      tmux.Target{Socket: "default", PaneID: "%5"},
			SessionName: "dev",
			WindowName:  "shell",
			CurrentCmd:  "codex",
			CurrentPath: "/home/gle/tmp/ext4/data/codedata/ai-hub/tmux-connect",
		},
		Metadata: tmux.BridgeMetadata{Managed: true, Agent: tmux.AgentCodex, Mode: tmux.ModeRelay},
	}
	return &fakePaneService{
		records: map[string]tmuxconn.PaneRecord{
			record.Info.Target.PaneKey(): record,
		},
		sub:           tmux.NewSubscriptionForTest(),
		snapshotText:  "hello from pane",
		snapshotRich:  "\x1b[32mhello from pane\x1b[0m",
		initialOutput: "initial output",
	}
}

func (s *fakePaneService) List(context.Context) ([]tmuxconn.PaneRecord, error) {
	s.listCalls++
	out := make([]tmuxconn.PaneRecord, 0, len(s.records))
	for _, record := range s.records {
		out = append(out, record)
	}
	return out, nil
}

func (s *fakePaneService) Attach(_ context.Context, ref string, agent string, _ string) (tmuxconn.PaneRecord, error) {
	s.attachCalls++
	key := normalizePaneRef(ref)
	record, ok := s.records[key]
	if !ok {
		return tmuxconn.PaneRecord{}, fmt.Errorf("pane not found: %s", ref)
	}
	record.Metadata.Managed = true
	record.Metadata.Mode = tmux.ModeRelay
	record.Metadata.Agent = tmux.NormalizeAgent(agent)
	s.records[key] = record
	return record, nil
}

func (s *fakePaneService) Detach(_ context.Context, ref string) error {
	s.detachCalls++
	key := normalizePaneRef(ref)
	record, ok := s.records[key]
	if !ok {
		return fmt.Errorf("pane not found: %s", ref)
	}
	record.Metadata = tmux.DefaultMetadata()
	s.records[key] = record
	return nil
}

func (s *fakePaneService) Inspect(_ context.Context, ref string) (tmuxconn.PaneRecord, error) {
	key := normalizePaneRef(ref)
	record, ok := s.records[key]
	if !ok {
		return tmuxconn.PaneRecord{}, fmt.Errorf("pane not found: %s", ref)
	}
	return record, nil
}

func (s *fakePaneService) Snapshot(context.Context, string, int) (string, error) {
	if s.snapshotCalls < len(s.snapshotSequence) {
		result := s.snapshotSequence[s.snapshotCalls]
		s.snapshotCalls++
		return result.body, result.err
	}
	s.snapshotCalls++
	return s.snapshotText, nil
}

func (s *fakePaneService) SnapshotRich(context.Context, string, int) (string, error) {
	return s.snapshotRich, nil
}

func (s *fakePaneService) Send(context.Context, string, string, bool) error { return nil }

func (s *fakePaneService) SendManaged(_ context.Context, paneKey string, text string, sendEnter bool) error {
	if s.sendErr != nil {
		return s.sendErr
	}
	s.sendCalls = append(s.sendCalls, sendCall{paneKey: paneKey, text: text, sendEnter: sendEnter})
	return nil
}

func (s *fakePaneService) SendKeys(context.Context, string, ...string) error { return nil }

func (s *fakePaneService) SendKeysManaged(_ context.Context, paneKey string, keys ...string) error {
	s.keyCalls = append(s.keyCalls, keyCall{paneKey: paneKey, keys: append([]string(nil), keys...)})
	return nil
}

func (s *fakePaneService) Enter(context.Context, string) error { return nil }

func (s *fakePaneService) EnterManaged(ctx context.Context, paneKey string) error {
	return s.SendKeysManaged(ctx, paneKey, "Enter")
}

func (s *fakePaneService) CtrlC(context.Context, string) error { return nil }

func (s *fakePaneService) CtrlCManaged(ctx context.Context, paneKey string) error {
	return s.SendKeysManaged(ctx, paneKey, "C-c")
}

func (s *fakePaneService) OpenStream(context.Context, string, int) (tmuxconn.PaneStream, error) {
	return tmuxconn.PaneStream{
		Pane:         s.records["default:%5"].Info,
		Initial:      s.initialOutput,
		Subscription: s.sub,
	}, nil
}

func normalizePaneRef(ref string) string {
	if strings.HasPrefix(ref, "%") {
		return "default:" + ref
	}
	return ref
}

func telegramChat(id int64) ChatRef {
	return ChatRef{Platform: "telegram", ChatID: strconv.FormatInt(id, 10)}
}

func slackChat(id string) ChatRef {
	return ChatRef{Platform: "slack", ChatID: id}
}

func feishuChat(id string) ChatRef {
	return ChatRef{Platform: "feishu", ChatID: id}
}

func discordChat(id string) ChatRef {
	return ChatRef{Platform: "discord", ChatID: id}
}

func whatsappChat(id string) ChatRef {
	return ChatRef{Platform: "whatsapp", ChatID: id}
}

func weixinChat(id string) ChatRef {
	return ChatRef{Platform: "weixin", ChatID: id}
}

func telegramMessage(chatID int64, messageID int64, text string) IncomingMessage {
	return IncomingMessage{
		Chat:      telegramChat(chatID),
		MessageID: strconv.FormatInt(messageID, 10),
		Text:      text,
		ChatType:  "private",
	}
}

func slackMessage(chatID string, messageID string, text string) IncomingMessage {
	return IncomingMessage{
		Chat:      slackChat(chatID),
		MessageID: messageID,
		Text:      text,
		ChatType:  "im",
		ThreadID:  messageID,
	}
}

func slackAppMentionMessage(chatID string, messageID string, text string) IncomingMessage {
	message := slackMessage(chatID, messageID, text)
	message.IsAppMention = true
	return message
}

func slackThreadMessage(chatID string, threadID string, messageID string, text string) IncomingMessage {
	return IncomingMessage{
		Chat:         slackChat(chatID),
		MessageID:    messageID,
		Text:         text,
		ChatType:     "channel",
		ThreadID:     threadID,
		PendingScope: threadID,
	}
}

func discordMessage(chatID string, messageID string, text string) IncomingMessage {
	return IncomingMessage{
		Chat:         discordChat(chatID),
		MessageID:    messageID,
		Text:         text,
		ThreadID:     chatID,
		PendingScope: chatID,
	}
}

func feishuPrivateMessage(chatID string, messageID string, text string) IncomingMessage {
	return IncomingMessage{
		Chat:      feishuChat(chatID),
		MessageID: messageID,
		Text:      text,
		ChatType:  "p2p",
	}
}

func feishuGroupMentionMessage(chatID string, messageID string, text string) IncomingMessage {
	return IncomingMessage{
		Chat:         feishuChat(chatID),
		MessageID:    messageID,
		Text:         text,
		ChatType:     "group",
		IsAppMention: true,
	}
}

func feishuThreadMentionMessage(chatID string, threadID string, messageID string, text string) IncomingMessage {
	return IncomingMessage{
		Chat:         feishuChat(chatID),
		MessageID:    messageID,
		Text:         text,
		ChatType:     "group",
		ThreadID:     threadID,
		PendingScope: threadID,
		IsAppMention: true,
	}
}

func whatsappMessage(chatID string, messageID string, text string) IncomingMessage {
	return IncomingMessage{
		Chat:      whatsappChat(chatID),
		MessageID: messageID,
		Text:      text,
		ChatType:  "private",
	}
}

func whatsappSelfMessage(chatID string, messageID string, text string) IncomingMessage {
	message := whatsappMessage(chatID, messageID, text)
	message.IsFromSelf = true
	return message
}

func weixinMessage(chatID string, messageID string, text string) IncomingMessage {
	return IncomingMessage{
		Chat:      weixinChat(chatID),
		MessageID: messageID,
		Text:      text,
		ChatType:  "private",
	}
}

func waitForMessages(t *testing.T, timeout time.Duration, predicate func([]sentMessage) bool, messenger *fakeMessenger) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		messages := messenger.snapshot()
		if predicate(messages) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("condition not met before timeout, messages = %#v", messages)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
