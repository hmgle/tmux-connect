package daemon

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"image/png"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hmgle/tmux-connect/internal/tagb"
	"github.com/hmgle/tmux-connect/internal/telegram"
	"github.com/hmgle/tmux-connect/internal/termrender"
	"github.com/hmgle/tmux-connect/internal/tmux"
)

type fakeMessenger struct {
	mu       sync.Mutex
	messages []sentMessage
	platform string
}

type sentMessage struct {
	Text             string
	Caption          string
	FileName         string
	Photo            []byte
	ParseMode        telegram.ParseMode
	ReplyToMessageID int64
	ReplyMarkup      any
	ThreadID         string
}

func (m *fakeMessenger) Platform() string {
	if strings.TrimSpace(m.platform) == "" {
		return "telegram"
	}
	return m.platform
}

func (m *fakeMessenger) SendMessage(_ context.Context, _ ChatRef, text string, opts SendOptions) (OutboundMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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
		Text:             text,
		ParseMode:        parseMode,
		ReplyToMessageID: replyTo,
		ReplyMarkup:      opts.ReplyMarkup,
		ThreadID:         opts.ThreadID,
	})
	return OutboundMessage{MessageID: strconv.Itoa(len(m.messages))}, nil
}

func (m *fakeMessenger) SendImage(_ context.Context, _ ChatRef, fileName string, photo []byte, caption string, opts SendOptions) (OutboundMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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
		Caption:          caption,
		FileName:         fileName,
		Photo:            append([]byte(nil), photo...),
		ParseMode:        parseMode,
		ReplyToMessageID: replyTo,
		ThreadID:         opts.ThreadID,
	})
	return OutboundMessage{MessageID: strconv.Itoa(len(m.messages))}, nil
}

func (m *fakeMessenger) DecorateMessage(kind string, text string, opts SendOptions) (string, SendOptions) {
	if m.Platform() == "slack" {
		return decorateSlackMessage(kind, text, opts)
	}
	return decorateTelegramMessage(kind, text, opts)
}

func (m *fakeMessenger) PromptOptions(message IncomingMessage, spec commandPromptSpec) SendOptions {
	if m.Platform() == "slack" {
		return SendOptions{ThreadID: message.replyThreadID()}
	}
	return SendOptions{
		ReplyToMessageID: message.MessageID,
		ReplyMarkup: telegram.ForceReply{
			ForceReply:            true,
			InputFieldPlaceholder: spec.Placeholder,
		},
	}
}

func (m *fakeMessenger) SnapshotCaption(paneKey string) string {
	return formatSnapshotCaption(paneKey)
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
	records       map[string]tagb.PaneRecord
	sub           *tmux.Subscription
	snapshotText  string
	snapshotRich  string
	initialOutput string
	sendCalls     []sendCall
	keyCalls      []keyCall
	listCalls     int
	attachCalls   int
	detachCalls   int
}

type sendCall struct {
	paneKey   string
	text      string
	sendEnter bool
}

type keyCall struct {
	paneKey string
	keys    []string
}

func newFakePaneService() *fakePaneService {
	record := tagb.PaneRecord{
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
		records: map[string]tagb.PaneRecord{
			record.Info.Target.PaneKey(): record,
		},
		sub:           tmux.NewSubscriptionForTest(),
		snapshotText:  "hello from pane",
		snapshotRich:  "\x1b[32mhello from pane\x1b[0m",
		initialOutput: "initial output",
	}
}

func (s *fakePaneService) List(context.Context) ([]tagb.PaneRecord, error) {
	s.listCalls++
	out := make([]tagb.PaneRecord, 0, len(s.records))
	for _, record := range s.records {
		out = append(out, record)
	}
	return out, nil
}

func TestRouterPanesRefreshesLiveStateAfterSelect(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	record := service.records["default:%5"]
	record.Metadata.Managed = false
	record.Metadata.Agent = tmux.AgentUnknown
	service.records["default:%5"] = record
	messenger := &fakeMessenger{}
	registry := NewPaneRegistry(service)
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, registry, store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if service.listCalls != 0 {
		t.Fatalf("expected select to avoid immediate registry refresh, got %d list calls", service.listCalls)
	}
	if service.attachCalls != 1 {
		t.Fatalf("attachCalls = %d, want 1", service.attachCalls)
	}

	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/panes")); err != nil {
		t.Fatalf("HandleMessage(panes) error = %v", err)
	}
	if service.listCalls != 1 {
		t.Fatalf("expected first panes to refresh registry, got %d list calls", service.listCalls)
	}
	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, "  Pane  Cmd    Dir           Where") {
		t.Fatalf("last panes message = %q, want column header", last.Text)
	}
	if !strings.Contains(last.Text, "> %5    codex  tmux-connect  dev/shell") {
		t.Fatalf("last panes message = %q, want current pane row", last.Text)
	}
	if last.ParseMode != telegram.ParseModeHTML {
		t.Fatalf("panes parse mode = %q, want HTML", last.ParseMode)
	}

	if err := router.HandleMessage(ctx, telegramMessage(7, 3, "/panes")); err != nil {
		t.Fatalf("HandleMessage(second panes) error = %v", err)
	}
	if service.listCalls != 2 {
		t.Fatalf("expected panes to refresh registry on every request, got %d list calls", service.listCalls)
	}
}

func (s *fakePaneService) Attach(_ context.Context, ref string, agent string, _ string) (tagb.PaneRecord, error) {
	s.attachCalls++
	key := normalizePaneRef(ref)
	record, ok := s.records[key]
	if !ok {
		return tagb.PaneRecord{}, fmt.Errorf("pane not found: %s", ref)
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

func (s *fakePaneService) Inspect(_ context.Context, ref string) (tagb.PaneRecord, error) {
	key := normalizePaneRef(ref)
	record, ok := s.records[key]
	if !ok {
		return tagb.PaneRecord{}, fmt.Errorf("pane not found: %s", ref)
	}
	return record, nil
}

func (s *fakePaneService) Snapshot(context.Context, string, int) (string, error) {
	return s.snapshotText, nil
}

func (s *fakePaneService) SnapshotRich(context.Context, string, int) (string, error) {
	return s.snapshotRich, nil
}

func (s *fakePaneService) Send(context.Context, string, string, bool) error { return nil }
func (s *fakePaneService) SendManaged(_ context.Context, paneKey string, text string, sendEnter bool) error {
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

func (s *fakePaneService) OpenStream(context.Context, string, int) (tagb.PaneStream, error) {
	return tagb.PaneStream{
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

func TestRouterSelectAndSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	current, err := store.CurrentPane(ctx, telegramChat(7))
	if err != nil {
		t.Fatalf("CurrentPane() error = %v", err)
	}
	if current != "default:%5" {
		t.Fatalf("current = %q, want %q", current, "default:%5")
	}

	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/snapshot")); err != nil {
		t.Fatalf("HandleMessage(snapshot) error = %v", err)
	}
	messages := messenger.snapshot()
	if len(messages) < 2 || len(messages[len(messages)-1].Photo) == 0 {
		t.Fatalf("unexpected messages %#v", messages)
	}
	if got := messages[len(messages)-1].Caption; got != "default:%5 snapshot" {
		t.Fatalf("snapshot caption = %q, want %q", got, "default:%5 snapshot")
	}
	if got := messages[len(messages)-1].FileName; got != "pane-snapshot.png" {
		t.Fatalf("snapshot filename = %q, want %q", got, "pane-snapshot.png")
	}
	if got := messages[len(messages)-1].ReplyToMessageID; got != 2 {
		t.Fatalf("snapshot reply_to = %d, want 2", got)
	}
}

func TestRouterSelectPromptsForPaneAndExecutesReply(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 10, "/select")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}

	messages := messenger.snapshot()
	prompt := messages[len(messages)-1]
	if prompt.Text != "Reply with the pane to select, for example %5 or default:%5." {
		t.Fatalf("prompt text = %q", prompt.Text)
	}
	if prompt.ReplyToMessageID != 10 {
		t.Fatalf("prompt reply_to = %d, want 10", prompt.ReplyToMessageID)
	}
	forceReply, ok := prompt.ReplyMarkup.(telegram.ForceReply)
	if !ok {
		t.Fatalf("prompt reply markup = %#v, want ForceReply", prompt.ReplyMarkup)
	}
	if !forceReply.ForceReply || forceReply.InputFieldPlaceholder != "%5" {
		t.Fatalf("force reply = %#v, want placeholder %%5", forceReply)
	}

	if err := router.HandleMessage(ctx, telegramMessage(7, 11, "%5")); err != nil {
		t.Fatalf("HandleMessage(pending select reply) error = %v", err)
	}

	current, err := store.CurrentPane(ctx, telegramChat(7))
	if err != nil {
		t.Fatalf("CurrentPane() error = %v", err)
	}
	if current != "default:%5" {
		t.Fatalf("current = %q, want %q", current, "default:%5")
	}
	latest := messenger.snapshot()
	last := latest[len(latest)-1]
	if !strings.Contains(last.Text, "selected default:%5") {
		t.Fatalf("last message = %q, want select confirmation", last.Text)
	}
}

func TestRouterSendPromptsForTextAndUsesReply(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/send")); err != nil {
		t.Fatalf("HandleMessage(send) error = %v", err)
	}
	latest := messenger.snapshot()
	prompt := latest[len(latest)-1]
	if prompt.Text != "Reply with the text to send to the current pane." {
		t.Fatalf("prompt text = %q", prompt.Text)
	}

	if err := router.HandleMessage(ctx, telegramMessage(7, 3, "status --short")); err != nil {
		t.Fatalf("HandleMessage(send reply) error = %v", err)
	}
	if len(service.sendCalls) != 1 {
		t.Fatalf("sendCalls = %#v, want one send", service.sendCalls)
	}
	if service.sendCalls[0].paneKey != "default:%5" || service.sendCalls[0].text != "status --short" {
		t.Fatalf("send call = %#v, want pane default:%%5 and text", service.sendCalls[0])
	}
	if service.sendCalls[0].sendEnter {
		t.Fatalf("send call = %#v, want sendEnter false", service.sendCalls[0])
	}
}

func TestRouterPlainTextSendsToCurrentPane(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "status --short")); err != nil {
		t.Fatalf("HandleMessage(plain text) error = %v", err)
	}
	if len(service.sendCalls) != 1 {
		t.Fatalf("sendCalls = %#v, want one send", service.sendCalls)
	}
	if service.sendCalls[0].paneKey != "default:%5" || service.sendCalls[0].text != "status --short" || service.sendCalls[0].sendEnter {
		t.Fatalf("send call = %#v, want plain text send without Enter", service.sendCalls[0])
	}
}

func TestRouterPlainTextWithoutCurrentPaneReturnsSelectError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "status --short")); err != nil {
		t.Fatalf("HandleMessage(plain text) error = %v", err)
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, "no current pane; run /select <pane> first") {
		t.Fatalf("last message = %q, want select error", last.Text)
	}
}

func TestRouterKeysSendsNormalizedTmuxKeys(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/keys ctrl-c enter esc")); err != nil {
		t.Fatalf("HandleMessage(keys) error = %v", err)
	}
	if len(service.keyCalls) != 1 {
		t.Fatalf("keyCalls = %#v, want one send", service.keyCalls)
	}
	got := service.keyCalls[0]
	if got.paneKey != "default:%5" {
		t.Fatalf("key call pane = %q, want %q", got.paneKey, "default:%5")
	}
	if strings.Join(got.keys, " ") != "C-c Enter Escape" {
		t.Fatalf("key call keys = %#v, want normalized keys", got.keys)
	}
}

func TestRouterKeysPromptUsesReply(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/keys")); err != nil {
		t.Fatalf("HandleMessage(keys) error = %v", err)
	}
	latest := messenger.snapshot()
	prompt := latest[len(latest)-1]
	if prompt.Text != "Reply with the tmux key names to send, for example C-c or Enter." {
		t.Fatalf("prompt text = %q", prompt.Text)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 3, "left tab")); err != nil {
		t.Fatalf("HandleMessage(keys reply) error = %v", err)
	}
	if len(service.keyCalls) != 1 {
		t.Fatalf("keyCalls = %#v, want one send", service.keyCalls)
	}
	if strings.Join(service.keyCalls[0].keys, " ") != "Left Tab" {
		t.Fatalf("key call keys = %#v, want normalized keys", service.keyCalls[0].keys)
	}
}

func TestRouterEnterAndCtrlCAliasKeySending(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/enter")); err != nil {
		t.Fatalf("HandleMessage(enter) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 3, "/ctrl-c")); err != nil {
		t.Fatalf("HandleMessage(ctrl-c) error = %v", err)
	}
	if len(service.keyCalls) != 2 {
		t.Fatalf("keyCalls = %#v, want enter and ctrl-c", service.keyCalls)
	}
	if strings.Join(service.keyCalls[0].keys, " ") != "Enter" {
		t.Fatalf("first key call = %#v, want Enter", service.keyCalls[0])
	}
	if strings.Join(service.keyCalls[1].keys, " ") != "C-c" {
		t.Fatalf("second key call = %#v, want C-c", service.keyCalls[1])
	}
}

func TestRouterNewCommandClearsPendingPrompt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/help")); err != nil {
		t.Fatalf("HandleMessage(help) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 3, "%5")); err != nil {
		t.Fatalf("HandleMessage(text after help) error = %v", err)
	}

	current, err := store.CurrentPane(ctx, telegramChat(7))
	if err != nil {
		t.Fatalf("CurrentPane() error = %v", err)
	}
	if current != "" {
		t.Fatalf("current = %q, want empty", current)
	}
	latest := messenger.snapshot()
	last := latest[len(latest)-1]
	if !strings.Contains(last.Text, "no current pane; run /select <pane> first") {
		t.Fatalf("last message = %q, want select error", last.Text)
	}
}

func TestRouterSelectAutoAttachesUnmanagedPane(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	record := service.records["default:%5"]
	record.Metadata.Managed = false
	record.Metadata.Agent = tmux.AgentUnknown
	service.records["default:%5"] = record
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if service.attachCalls != 1 {
		t.Fatalf("attachCalls = %d, want 1", service.attachCalls)
	}

	current, err := store.CurrentPane(ctx, telegramChat(7))
	if err != nil {
		t.Fatalf("CurrentPane() error = %v", err)
	}
	if current != "default:%5" {
		t.Fatalf("current = %q, want %q", current, "default:%5")
	}

	record, err = service.Inspect(ctx, "%5")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if !record.Metadata.Managed {
		t.Fatalf("managed = false, want true")
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, "selected default:%5") {
		t.Fatalf("last message = %q, want select confirmation", last.Text)
	}
}

func TestFormatPaneListShortensLongDirectoryNames(t *testing.T) {
	t.Parallel()

	text := formatPaneList([]tagb.PaneRecord{{
		Info: tmux.PaneInfo{
			Target:      tmux.Target{Socket: "default", PaneID: "%7"},
			SessionName: "workspace",
			WindowName:  "review",
			CurrentCmd:  "claude",
			CurrentPath: "/srv/very-long-directory-name-for-agents",
		},
	}}, "default:%7", false)

	if !strings.Contains(text, "> %7    claude  very-...agents  workspace/review") {
		t.Fatalf("formatPaneList() = %q, want padded row with shortened directory", text)
	}
	if !strings.Contains(text, "Current: %7 · Follow: off") {
		t.Fatalf("formatPaneList() = %q, want summary line", text)
	}
}

func TestRouterSnapshotTextMode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/snapshot text")); err != nil {
		t.Fatalf("HandleMessage(snapshot text) error = %v", err)
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if len(last.Photo) != 0 {
		t.Fatalf("expected text snapshot, got photo message %#v", last)
	}
	if !strings.Contains(last.Text, "hello from pane") {
		t.Fatalf("snapshot text = %q, want pane text", last.Text)
	}
	if got := last.ParseMode; got != telegram.ParseModeHTML {
		t.Fatalf("snapshot text parse mode = %q, want %q", got, telegram.ParseModeHTML)
	}
}

func TestReplyBusReplySnapshotUsesConfiguredRenderOptions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{
		ThemeName: termrender.ThemeLight,
		FontSize:  20,
	})

	if err := replyBus.ReplySnapshot(ctx, telegramChat(7), "default:%5", "plain text", "\x1b[31merror\x1b[0m"); err != nil {
		t.Fatalf("ReplySnapshot() error = %v", err)
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if len(last.Photo) == 0 {
		t.Fatalf("expected photo message, got %#v", last)
	}

	img, err := png.Decode(bytes.NewReader(last.Photo))
	if err != nil {
		t.Fatalf("png decode error = %v", err)
	}
	got := color.RGBAModel.Convert(img.At(0, 0)).(color.RGBA)
	want := color.RGBA{R: 248, G: 250, B: 252, A: 255}
	if got != want {
		t.Fatalf("background = %#v, want %#v", got, want)
	}
}

func TestRouterFollow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	follow := NewFollowManager(service, replyBus, 20)
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, follow, 120, nil, "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/follow on")); err != nil {
		t.Fatalf("HandleMessage(follow on) error = %v", err)
	}

	service.sub.PushChunk(tmux.OutputChunk{Text: "delta output", ReceivedAt: time.Now()})
	waitForMessages(t, time.Second, func(messages []sentMessage) bool {
		for _, msg := range messages {
			if strings.Contains(msg.Text, "delta output") {
				return true
			}
		}
		return false
	}, messenger)
	follow.Disable(telegramChat(7).Key())

	messages := messenger.snapshot()
	texts := make([]string, 0, len(messages))
	for _, msg := range messages {
		texts = append(texts, msg.Text)
	}
	joined := strings.Join(texts, "\n")
	if !strings.Contains(joined, "follow enabled") {
		t.Fatalf("missing follow confirmation in %q", joined)
	}
	if !strings.Contains(joined, "initial output") {
		t.Fatalf("missing initial output in %q", joined)
	}
	if !strings.Contains(joined, "delta output") {
		t.Fatalf("missing streamed output in %q", joined)
	}
	for _, msg := range messages {
		if strings.Contains(msg.Text, "initial output") || strings.Contains(msg.Text, "delta output") {
			if msg.ParseMode != telegram.ParseModeHTML {
				t.Fatalf("follow output parse mode = %q, want %q for %#v", msg.ParseMode, telegram.ParseModeHTML, msg)
			}
		}
	}
	for _, msg := range messages[1:] {
		if msg.ReplyToMessageID != 2 {
			t.Fatalf("follow reply_to = %d, want 2 for %#v", msg.ReplyToMessageID, msg)
		}
	}
}

func TestRouterFollowFlushesBufferedOutputOnDisable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	follow := NewFollowManager(service, replyBus, 20)
	follow.minInterval = 5 * time.Second
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, follow, 120, nil, "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/follow on")); err != nil {
		t.Fatalf("HandleMessage(follow on) error = %v", err)
	}

	service.sub.PushChunk(tmux.OutputChunk{Text: "buffered before disable", ReceivedAt: time.Now()})
	if !follow.Disable(telegramChat(7).Key()) {
		t.Fatalf("Disable() = false, want true")
	}

	waitForMessages(t, time.Second, func(messages []sentMessage) bool {
		for _, msg := range messages {
			if strings.Contains(msg.Text, "buffered before disable") {
				return true
			}
		}
		return false
	}, messenger)
}

func TestRouterFollowSupportsCustomInterval(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	follow := NewFollowManager(service, replyBus, 20)
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, follow, 120, nil, "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/follow on 2s")); err != nil {
		t.Fatalf("HandleMessage(follow on 2s) error = %v", err)
	}

	if got := follow.Options(telegramChat(7).Key()).MinInterval; got != 2*time.Second {
		t.Fatalf("follow interval = %s, want %s", got, 2*time.Second)
	}

	messages := messenger.snapshot()
	found := false
	for _, msg := range messages {
		if strings.Contains(msg.Text, "min interval 2s") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("unexpected follow confirmation %#v", messages)
	}
}

func TestRouterFollowShowsContextForInlineUpdates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	service.initialOutput = "ready\ncalc> "
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	follow := NewFollowManager(service, replyBus, 20)
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, follow, 120, nil, "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/follow on 10ms")); err != nil {
		t.Fatalf("HandleMessage(follow on 10ms) error = %v", err)
	}

	service.sub.PushChunk(tmux.OutputChunk{Text: "1", ReceivedAt: time.Now()})
	waitForMessages(t, time.Second, func(messages []sentMessage) bool {
		for _, msg := range messages {
			if strings.Contains(msg.Text, "calc&gt; 1") {
				return true
			}
		}
		return false
	}, messenger)

	service.sub.PushChunk(tmux.OutputChunk{Text: "+", ReceivedAt: time.Now()})
	waitForMessages(t, time.Second, func(messages []sentMessage) bool {
		for _, msg := range messages {
			if strings.Contains(msg.Text, "calc&gt; 1+") {
				return true
			}
		}
		return false
	}, messenger)

	service.sub.PushChunk(tmux.OutputChunk{Text: "2", ReceivedAt: time.Now()})
	waitForMessages(t, time.Second, func(messages []sentMessage) bool {
		for _, msg := range messages {
			if strings.Contains(msg.Text, "calc&gt; 1+2") {
				return true
			}
		}
		return false
	}, messenger)

	messages := messenger.snapshot()
	latest := messages[len(messages)-1].Text
	if !strings.Contains(latest, "ready") {
		t.Fatalf("latest message = %q, want previous context", latest)
	}
	if !strings.Contains(latest, "calc&gt; 1+2") {
		t.Fatalf("latest message = %q, want complete updated line", latest)
	}
	if strings.HasSuffix(latest, "\n1") || strings.HasSuffix(latest, "\n+") || strings.HasSuffix(latest, "\n2") {
		t.Fatalf("latest message = %q, want contextual line instead of raw character delta", latest)
	}
}

func TestRouterFollowDrainsChunksAfterErrChannelCloses(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	follow := NewFollowManager(service, replyBus, 20)
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, follow, 120, nil, "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/follow on")); err != nil {
		t.Fatalf("HandleMessage(follow on) error = %v", err)
	}

	service.sub.CloseErrs()
	service.sub.PushChunk(tmux.OutputChunk{Text: "chunk after errs close", ReceivedAt: time.Now()})
	service.sub.CloseChunks()

	waitForMessages(t, time.Second, func(messages []sentMessage) bool {
		for _, msg := range messages {
			if strings.Contains(msg.Text, "chunk after errs close") {
				return true
			}
		}
		return false
	}, messenger)
}

func TestRouterClearStopsFollowAndKeepsSelectionHistory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	follow := NewFollowManager(service, replyBus, 20)
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, follow, 120, nil, "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/follow on")); err != nil {
		t.Fatalf("HandleMessage(follow on) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 3, "/clear")); err != nil {
		t.Fatalf("HandleMessage(clear) error = %v", err)
	}

	current, err := store.CurrentPane(ctx, telegramChat(7))
	if err != nil {
		t.Fatalf("CurrentPane() error = %v", err)
	}
	if current != "" {
		t.Fatalf("current = %q, want empty", current)
	}

	bindings, err := store.ListBindings(ctx, telegramChat(7))
	if err != nil {
		t.Fatalf("ListBindings() error = %v", err)
	}
	if len(bindings) != 1 || bindings[0] != "default:%5" {
		t.Fatalf("bindings = %#v, want [default:%%5]", bindings)
	}
	if follow.IsEnabled(telegramChat(7).Key()) {
		t.Fatalf("follow is still enabled")
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, "cleared current pane") {
		t.Fatalf("last message = %q, want clear confirmation", last.Text)
	}
}

func TestRouterUnmanageClearsBindingsAndFollow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	follow := NewFollowManager(service, replyBus, 20)
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, follow, 120, nil, "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/follow on")); err != nil {
		t.Fatalf("HandleMessage(follow on) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 3, "/unmanage %5")); err != nil {
		t.Fatalf("HandleMessage(unmanage) error = %v", err)
	}

	if service.detachCalls != 1 {
		t.Fatalf("detachCalls = %d, want 1", service.detachCalls)
	}

	current, err := store.CurrentPane(ctx, telegramChat(7))
	if err != nil {
		t.Fatalf("CurrentPane() error = %v", err)
	}
	if current != "" {
		t.Fatalf("current = %q, want empty", current)
	}

	bindings, err := store.ListBindings(ctx, telegramChat(7))
	if err != nil {
		t.Fatalf("ListBindings() error = %v", err)
	}
	if len(bindings) != 0 {
		t.Fatalf("bindings = %#v, want empty", bindings)
	}
	if follow.IsEnabled(telegramChat(7).Key()) {
		t.Fatalf("follow is still enabled")
	}

	record, err := service.Inspect(ctx, "%5")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if record.Metadata.Managed {
		t.Fatalf("managed = true, want false")
	}
}

func TestHelpTextUsesNewTelegramCommands(t *testing.T) {
	t.Parallel()

	text := helpText("")
	for _, want := range []string{"/start", "/help", "/select <pane>", "/clear", "/unmanage <pane>"} {
		if !strings.Contains(text, want) {
			t.Fatalf("helpText() missing %q in %q", want, text)
		}
	}
	for _, old := range []string{"/bind <pane>", "/attach <pane>", "/detach <pane>"} {
		if strings.Contains(text, old) {
			t.Fatalf("helpText() unexpectedly contains %q in %q", old, text)
		}
	}
}

func TestHelpTextUsesSlackPrefixedCommands(t *testing.T) {
	t.Parallel()

	text := helpText("tmux:")
	for _, want := range []string{"\ntmux: start", "\ntmux: help", "\ntmux: select <pane>", "\ntmux: snapshot [lines] [image|text]"} {
		if !strings.Contains(text, want) {
			t.Fatalf("helpText(tmux:) missing %q in %q", want, text)
		}
	}
	for _, unwanted := range []string{"/start", "\nstart", "\nselect <pane>", "\nsnapshot [lines] [image|text]"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("helpText(tmux:) unexpectedly contains %q in %q", unwanted, text)
		}
	}
}

func TestCommandSpecsMatchHelp(t *testing.T) {
	t.Parallel()

	commands := daemonCommandSpecs()
	if len(commands) == 0 {
		t.Fatal("daemonCommandSpecs() returned no commands")
	}
	if commands[0].Command != "start" || commands[0].Description == "" {
		t.Fatalf("first command = %#v, want start with description", commands[0])
	}
	if commands[len(commands)-1].Command != "follow" {
		t.Fatalf("last command = %#v, want follow", commands[len(commands)-1])
	}
	telegramHelp := helpText("")
	slackHelp := helpText("tmux:")
	for _, command := range commands {
		if !strings.Contains(telegramHelp, formatCommandUsage("", command.Usage)) {
			t.Fatalf("helpText (telegram) missing %q", formatCommandUsage("", command.Usage))
		}
		if !strings.Contains(slackHelp, formatCommandUsage("tmux:", command.Usage)) {
			t.Fatalf("helpText (slack) missing %q", formatCommandUsage("tmux:", command.Usage))
		}
	}
}

func TestRouterSlackAcceptsPrefixedCommands(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "slack"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "")

	if err := router.HandleMessage(ctx, slackMessage("D123", "1", "tmux: select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, slackMessage("D123", "2", "tmux: current")); err != nil {
		t.Fatalf("HandleMessage(current) error = %v", err)
	}

	current, err := store.CurrentPane(ctx, slackChat("D123"))
	if err != nil {
		t.Fatalf("CurrentPane() error = %v", err)
	}
	if current != "default:%5" {
		t.Fatalf("current = %q, want default:%%5", current)
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, "Current pane:") {
		t.Fatalf("last message = %q, want current pane details", last.Text)
	}
}

func TestRouterSlackAppMentionAcceptsCommandWithoutPrefix(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "slack"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "")

	if err := router.HandleMessage(ctx, slackAppMentionMessage("C123", "1", "panes")); err != nil {
		t.Fatalf("HandleMessage(app mention panes) error = %v", err)
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, "Pane") {
		t.Fatalf("last message = %q, want pane list", last.Text)
	}
}

func TestRouterSlackHelpUsesCommandPrefix(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "slack"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "")

	if err := router.HandleMessage(ctx, slackMessage("D123", "1", "tmux: help")); err != nil {
		t.Fatalf("HandleMessage(help) error = %v", err)
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, `prefix commands with "tmux:"`) {
		t.Fatalf("last message = %q, want slack hint", last.Text)
	}
	if !strings.Contains(last.Text, "plain text sends to the current pane") {
		t.Fatalf("last message = %q, want plain text hint", last.Text)
	}
	if !strings.Contains(last.Text, "tmux: keys C-c") {
		t.Fatalf("last message = %q, want keys example", last.Text)
	}
	if strings.Contains(last.Text, "/select <pane>") {
		t.Fatalf("last message = %q, want tmux:-prefixed slack commands", last.Text)
	}
	if !strings.Contains(last.Text, "tmux: select <pane>") {
		t.Fatalf("last message = %q, want prefixed select usage", last.Text)
	}
}

func TestRouterSlackPlainTextUsesCurrentPane(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "slack"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "")

	if err := router.HandleMessage(ctx, slackMessage("D123", "1", "tmux: select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, slackMessage("D123", "2", "tmux panes")); err != nil {
		t.Fatalf("HandleMessage(plain text) error = %v", err)
	}
	if len(service.sendCalls) != 1 {
		t.Fatalf("sendCalls = %#v, want one plain text send", service.sendCalls)
	}
	if service.sendCalls[0].text != "tmux panes" || service.sendCalls[0].sendEnter {
		t.Fatalf("send call = %#v, want plain text send without Enter", service.sendCalls[0])
	}
}

func TestRouterSlackPlainTextInManagedThreadUsesCurrentPane(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "slack"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "")

	if err := router.HandleMessage(ctx, slackAppMentionMessage("C123", "1", "select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, slackThreadMessage("C123", "1", "2", "status --short")); err != nil {
		t.Fatalf("HandleMessage(thread plain text) error = %v", err)
	}
	if len(service.sendCalls) != 1 {
		t.Fatalf("sendCalls = %#v, want one plain text send", service.sendCalls)
	}
	if service.sendCalls[0].paneKey != "default:%5" || service.sendCalls[0].text != "status --short" {
		t.Fatalf("send call = %#v, want thread plain text send", service.sendCalls[0])
	}
}

func TestRouterSlackPlainTextWithoutCurrentPaneReturnsSelectError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tagb.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "slack"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "")

	if err := router.HandleMessage(ctx, slackMessage("D123", "1", "tmux panes")); err != nil {
		t.Fatalf("HandleMessage(plain text) error = %v", err)
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, "no current pane; run tmux: select <pane> first") {
		t.Fatalf("last message = %q, want select error", last.Text)
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

func TestParseKeysArgs(t *testing.T) {
	t.Parallel()

	keys, err := parseKeysArgs("ctrl-c enter esc left tab")
	if err != nil {
		t.Fatalf("parseKeysArgs() error = %v", err)
	}
	if strings.Join(keys, " ") != "C-c Enter Escape Left Tab" {
		t.Fatalf("keys = %#v, want normalized key sequence", keys)
	}

	if _, err := parseKeysArgs("   "); err == nil {
		t.Fatal("parseKeysArgs(empty) error = nil, want error")
	}
}

func TestParseSnapshotArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		value     string
		wantLines int
		wantMode  snapshotMode
		wantErr   bool
	}{
		{name: "default", value: "", wantLines: 120, wantMode: snapshotModeImage},
		{name: "lines only", value: "200", wantLines: 200, wantMode: snapshotModeImage},
		{name: "text only", value: "text", wantLines: 120, wantMode: snapshotModeText},
		{name: "image only", value: "image", wantLines: 120, wantMode: snapshotModeImage},
		{name: "lines then text", value: "200 text", wantLines: 200, wantMode: snapshotModeText},
		{name: "text then lines", value: "text 200", wantLines: 200, wantMode: snapshotModeText},
		{name: "bad mode", value: "plain", wantErr: true},
		{name: "bad lines", value: "0", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines, mode, err := parseSnapshotArgs(tt.value, 120)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseSnapshotArgs(%q) error = nil, want error", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSnapshotArgs(%q) error = %v", tt.value, err)
			}
			if lines != tt.wantLines {
				t.Fatalf("lines = %d, want %d", lines, tt.wantLines)
			}
			if mode != tt.wantMode {
				t.Fatalf("mode = %q, want %q", mode, tt.wantMode)
			}
		})
	}
}
