package daemon

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hmgle/tmux-connect/internal/termrender"
)

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

func TestHelpTextUsesDiscordSlashCommands(t *testing.T) {
	t.Parallel()

	text := helpTextForPlatform("discord", "", "tmux:")
	for _, want := range []string{"/start", "/help", "/snapshot [lines] [image|text]", `prefix text commands with "tmux:"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("helpTextForPlatform(discord) missing %q in %q", want, text)
		}
	}
	for _, unwanted := range []string{"\ntmux: start", "\ntmux: help"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("helpTextForPlatform(discord) unexpectedly contains %q in %q", unwanted, text)
		}
	}
}

func TestHelpTextUsesWhatsAppReplyGuidance(t *testing.T) {
	t.Parallel()

	text := helpTextForPlatform("whatsapp", "", "tmux:")
	for _, want := range []string{`"/panes"`, `"/follow on"`, `replying with "1" or "2"`, `plain text is disabled to avoid reply loops`} {
		if !strings.Contains(text, want) {
			t.Fatalf("helpTextForPlatform(whatsapp) missing %q in %q", want, text)
		}
	}
}

func TestHelpTextUsesFeishuGuidance(t *testing.T) {
	t.Parallel()

	text := helpTextForPlatform("feishu", "", "tmux:")
	for _, want := range []string{`Feishu private chats`, `mention the bot`, `Static cards are used for help and pane selection prompts`} {
		if !strings.Contains(text, want) {
			t.Fatalf("helpTextForPlatform(feishu) missing %q in %q", want, text)
		}
	}
}

func TestRouterWhatsAppSelfChatRejectsPlainText(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "whatsapp"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouterWithPlainTextConfig(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "", PlainTextConfig{
		WhatsAppSelfChatCommandOnly: true,
	})

	if err := router.HandleMessage(ctx, whatsappMessage("8613800000000@s.whatsapp.net", "wamid-1", "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, whatsappSelfMessage("8613800000000@s.whatsapp.net", "wamid-2", "pwd")); err != nil {
		t.Fatalf("HandleMessage(self plain text) error = %v", err)
	}
	if len(service.sendCalls) != 0 {
		t.Fatalf("sendCalls = %#v, want no tmux input for self-chat plain text", service.sendCalls)
	}
	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, "WhatsApp self-chat disables plain text") {
		t.Fatalf("last message = %q, want self-chat guidance", last.Text)
	}
}

func TestRouterWhatsAppSelfChatPromptReplyStillWorks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	record := service.records["default:%5"]
	record2 := record
	record2.Info.Target.PaneID = "%7"
	service.records[record2.Info.Target.PaneKey()] = record2
	messenger := &fakeMessenger{platform: "whatsapp"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouterWithPlainTextConfig(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "", PlainTextConfig{
		WhatsAppSelfChatCommandOnly: true,
	})

	if err := router.HandleMessage(ctx, whatsappSelfMessage("8613800000000@s.whatsapp.net", "wamid-1", "/select")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, whatsappSelfMessage("8613800000000@s.whatsapp.net", "wamid-2", "2")); err != nil {
		t.Fatalf("HandleMessage(select reply) error = %v", err)
	}
	current, err := store.CurrentPane(ctx, whatsappChat("8613800000000@s.whatsapp.net"))
	if err != nil {
		t.Fatalf("CurrentPane() error = %v", err)
	}
	if current != "default:%7" {
		t.Fatalf("current = %q, want default:%%7", current)
	}
}

func TestRouterWhatsAppSelectPromptSupportsNumericReply(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	record := service.records["default:%5"]
	record2 := record
	record2.Info.Target.PaneID = "%7"
	service.records[record2.Info.Target.PaneKey()] = record2
	messenger := &fakeMessenger{platform: "whatsapp"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

	if err := router.HandleMessage(ctx, whatsappMessage("8613800000000@s.whatsapp.net", "wamid-1", "/select")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	prompt := messenger.snapshot()[0]
	if !strings.Contains(prompt.Text, "1. default:%5") || !strings.Contains(prompt.Text, "2. default:%7") {
		t.Fatalf("prompt text = %q, want numbered pane options", prompt.Text)
	}
	if prompt.ReplyToMessageID != 0 {
		t.Fatalf("prompt reply_to = %d, want whatsapp adapter to leave numeric parse in text only", prompt.ReplyToMessageID)
	}

	if err := router.HandleMessage(ctx, whatsappMessage("8613800000000@s.whatsapp.net", "wamid-2", "2")); err != nil {
		t.Fatalf("HandleMessage(select reply) error = %v", err)
	}
	current, err := store.CurrentPane(ctx, whatsappChat("8613800000000@s.whatsapp.net"))
	if err != nil {
		t.Fatalf("CurrentPane() error = %v", err)
	}
	if current != "default:%7" {
		t.Fatalf("current = %q, want default:%%7", current)
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
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "slack"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

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

func TestRouterFeishuPrivatePlainTextUsesCurrentPane(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "feishu"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

	if err := router.HandleMessage(ctx, feishuPrivateMessage("oc_chat_1", "m1", "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, feishuPrivateMessage("oc_chat_1", "m2", "pwd")); err != nil {
		t.Fatalf("HandleMessage(plain text) error = %v", err)
	}
	if len(service.sendCalls) != 1 {
		t.Fatalf("sendCalls len = %d, want 1", len(service.sendCalls))
	}
	if service.sendCalls[0].text != "pwd" {
		t.Fatalf("send text = %q, want pwd", service.sendCalls[0].text)
	}
}

func TestRouterFeishuGroupIgnoresNonMentionPlainText(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "feishu"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

	if err := router.HandleMessage(ctx, IncomingMessage{
		Chat:      feishuChat("oc_group_1"),
		MessageID: "m1",
		Text:      "pwd",
		ChatType:  "group",
	}); err != nil {
		t.Fatalf("HandleMessage(group plain text) error = %v", err)
	}
	if len(service.sendCalls) != 0 {
		t.Fatalf("sendCalls = %#v, want none", service.sendCalls)
	}
	if len(messenger.snapshot()) != 0 {
		t.Fatalf("messages = %#v, want none", messenger.snapshot())
	}
}

func TestRouterFeishuGroupMentionAcceptsCommand(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "feishu"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

	if err := router.HandleMessage(ctx, feishuGroupMentionMessage("oc_group_1", "m1", "select %5")); err != nil {
		t.Fatalf("HandleMessage(group mention select) error = %v", err)
	}
	current, err := store.CurrentPane(ctx, feishuChat("oc_group_1"))
	if err != nil {
		t.Fatalf("CurrentPane() error = %v", err)
	}
	if current != "default:%5" {
		t.Fatalf("current = %q, want default:%%5", current)
	}
}

func TestRouterFeishuGroupMentionOnlyShowsHelpCard(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "feishu"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

	if err := router.HandleMessage(ctx, feishuGroupMentionMessage("oc_group_1", "m1", "help")); err != nil {
		t.Fatalf("HandleMessage(group mention help) error = %v", err)
	}
	messages := messenger.snapshot()
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	if messages[0].Card == nil {
		t.Fatal("card = nil, want help card")
	}
}

func TestRouterFeishuHelpUsesCard(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "feishu"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

	if err := router.HandleMessage(ctx, feishuPrivateMessage("oc_chat_1", "m1", "/help")); err != nil {
		t.Fatalf("HandleMessage(help) error = %v", err)
	}
	messages := messenger.snapshot()
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	if messages[0].Card == nil {
		t.Fatal("card = nil, want feishu help card")
	}
}

func TestRouterAllowedChecksPlatformScopedAllowlist(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, []string{"discord:C123"}, "", "")

	if !router.allowed(discordChat("C123")) {
		t.Fatal("router.allowed(discord:C123) = false, want true")
	}
	if router.allowed(slackChat("C123")) {
		t.Fatal("router.allowed(slack:C123) = true, want false")
	}
}

func TestRouterDiscordPromptMentionsPrefixedReplyInChannels(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "discord"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "tmux:")

	if err := router.HandleMessage(ctx, discordMessage("C123", "1", "/select")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, `reply with "tmux: <value>"`) {
		t.Fatalf("prompt text = %q, want Discord channel reply hint", last.Text)
	}
}

func TestRouterSlackAppMentionAcceptsCommandWithoutPrefix(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "slack"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

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
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "slack"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

	if err := router.HandleMessage(ctx, slackMessage("D123", "1", "tmux: help")); err != nil {
		t.Fatalf("HandleMessage(help) error = %v", err)
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, `prefix commands with "tmux:"`) {
		t.Fatalf("last message = %q, want slack hint", last.Text)
	}
	if !strings.Contains(last.Text, "plain text targets the current pane") {
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
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "slack"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

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
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "slack"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

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

func TestRouterSlackPlainTextExecuteInManagedThreadReturnsSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	service.snapshotSequence = []snapshotResult{{body: "before"}, {body: "after\nok"}}
	messenger := &fakeMessenger{platform: "slack"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouterWithPlainTextConfig(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "", PlainTextConfig{
		Mode:        plainTextModeExecute,
		Echo:        plainTextEchoSnapshot,
		EchoLines:   8,
		EchoDelay:   time.Millisecond,
		EchoTimeout: 75 * time.Millisecond,
	})

	if err := router.HandleMessage(ctx, slackAppMentionMessage("C123", "1", "select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, slackThreadMessage("C123", "1", "2", "status --short")); err != nil {
		t.Fatalf("HandleMessage(thread plain text execute) error = %v", err)
	}
	if len(service.sendCalls) != 1 || !service.sendCalls[0].sendEnter {
		t.Fatalf("sendCalls = %#v, want one execute send", service.sendCalls)
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, "```") || !strings.Contains(last.Text, "after\nok") {
		t.Fatalf("last message = %#v, want slack code block snapshot", last)
	}
}

func TestRouterDiscordDMPlainTextExecuteReturnsSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	service.snapshotSequence = []snapshotResult{{body: "before"}, {body: "after\nok"}}
	messenger := &fakeMessenger{platform: "discord"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouterWithPlainTextConfig(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "", PlainTextConfig{
		Mode:        plainTextModeExecute,
		Echo:        plainTextEchoSnapshot,
		EchoLines:   8,
		EchoDelay:   time.Millisecond,
		EchoTimeout: 75 * time.Millisecond,
	})

	if err := router.HandleMessage(ctx, discordMessage("D123", "1", "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, discordMessage("D123", "2", "status --short")); err != nil {
		t.Fatalf("HandleMessage(discord plain text execute) error = %v", err)
	}
	if len(service.sendCalls) != 1 || !service.sendCalls[0].sendEnter {
		t.Fatalf("sendCalls = %#v, want one execute send", service.sendCalls)
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if last.Embed == nil || !strings.Contains(last.Embed.Description, "after\nok") {
		t.Fatalf("last message = %#v, want discord snapshot embed", last)
	}
}

func TestRouterSlackPlainTextWithoutCurrentPaneReturnsSelectError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "slack"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

	if err := router.HandleMessage(ctx, slackMessage("D123", "1", "tmux panes")); err != nil {
		t.Fatalf("HandleMessage(plain text) error = %v", err)
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, "no current pane; run tmux: select <pane> first") {
		t.Fatalf("last message = %q, want select error", last.Text)
	}
}
