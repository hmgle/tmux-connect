package daemon

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"image/png"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hmgle/tmux-connect/internal/telegram"
	"github.com/hmgle/tmux-connect/internal/termrender"
	"github.com/hmgle/tmux-connect/internal/tmux"
	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

func TestRouterPanesRefreshesLiveStateAfterSelect(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
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
	router := NewRouter(service, registry, store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

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

func TestRouterSelectAndSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

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
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

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
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

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
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

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

func TestRouterPlainTextExecuteSendsTextAndReturnsSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	service.snapshotSequence = []snapshotResult{{body: "before"}, {body: "before"}, {body: "after\nok"}}
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouterWithPlainTextConfig(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "", PlainTextConfig{
		Mode:        plainTextModeExecute,
		Echo:        plainTextEchoSnapshot,
		EchoLines:   8,
		EchoDelay:   time.Millisecond,
		EchoTimeout: 75 * time.Millisecond,
	})

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "status --short")); err != nil {
		t.Fatalf("HandleMessage(plain text execute) error = %v", err)
	}
	if len(service.sendCalls) != 1 {
		t.Fatalf("sendCalls = %#v, want one send", service.sendCalls)
	}
	if service.sendCalls[0].paneKey != "default:%5" || service.sendCalls[0].text != "status --short" || !service.sendCalls[0].sendEnter {
		t.Fatalf("send call = %#v, want plain text execute with Enter", service.sendCalls[0])
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, "[default:%5]") || !strings.Contains(last.Text, "after\nok") {
		t.Fatalf("last message = %q, want execute snapshot", last.Text)
	}
}

func TestRouterPlainTextExecuteTimeoutReturnsNoVisibleOutputMessage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	service.snapshotText = "same"
	service.snapshotSequence = []snapshotResult{{body: "same"}, {body: "same"}, {body: "same"}}
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouterWithPlainTextConfig(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "", PlainTextConfig{
		Mode:        plainTextModeExecute,
		Echo:        plainTextEchoSnapshot,
		EchoLines:   8,
		EchoDelay:   time.Millisecond,
		EchoTimeout: 10 * time.Millisecond,
	})

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "status --short")); err != nil {
		t.Fatalf("HandleMessage(plain text execute) error = %v", err)
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, "no visible output yet") {
		t.Fatalf("last message = %q, want no visible output fallback", last.Text)
	}
	if !strings.Contains(last.Text, "/snapshot text") || !strings.Contains(last.Text, "/follow on") {
		t.Fatalf("last message = %q, want snapshot and follow hints", last.Text)
	}
}

func TestRouterPlainTextExecuteSnapshotFailureReturnsErrorAfterSend(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	service.snapshotSequence = []snapshotResult{{err: fmt.Errorf("tmux down")}}
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouterWithPlainTextConfig(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "", PlainTextConfig{
		Mode:        plainTextModeExecute,
		Echo:        plainTextEchoSnapshot,
		EchoLines:   8,
		EchoDelay:   time.Millisecond,
		EchoTimeout: 75 * time.Millisecond,
	})

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "status --short")); err != nil {
		t.Fatalf("HandleMessage(plain text execute) error = %v", err)
	}
	if len(service.sendCalls) != 1 || !service.sendCalls[0].sendEnter {
		t.Fatalf("sendCalls = %#v, want command sent before snapshot error", service.sendCalls)
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, "snapshot failed") || !strings.Contains(last.Text, "tmux down") {
		t.Fatalf("last message = %q, want snapshot failure", last.Text)
	}
}

func TestRouterPlainTextExecuteSendFailureReturnsError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	service.sendErr = fmt.Errorf("write failed")
	service.snapshotSequence = []snapshotResult{{body: "before"}}
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouterWithPlainTextConfig(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "", PlainTextConfig{
		Mode:        plainTextModeExecute,
		Echo:        plainTextEchoSnapshot,
		EchoLines:   8,
		EchoDelay:   time.Millisecond,
		EchoTimeout: 75 * time.Millisecond,
	})

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "status --short")); err != nil {
		t.Fatalf("HandleMessage(plain text execute) error = %v", err)
	}
	if len(service.sendCalls) != 0 {
		t.Fatalf("sendCalls = %#v, want no recorded send on failure", service.sendCalls)
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, "send failed") || !strings.Contains(last.Text, "write failed") {
		t.Fatalf("last message = %q, want send failure", last.Text)
	}
}

func TestRouterPlainTextWithoutCurrentPaneReturnsSelectError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

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
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/keys ctrl-c enter esc ppage f2 m-enter s-left c-space kp0")); err != nil {
		t.Fatalf("HandleMessage(keys) error = %v", err)
	}
	if len(service.keyCalls) != 1 {
		t.Fatalf("keyCalls = %#v, want one send", service.keyCalls)
	}
	got := service.keyCalls[0]
	if got.paneKey != "default:%5" {
		t.Fatalf("key call pane = %q, want %q", got.paneKey, "default:%5")
	}
	if strings.Join(got.keys, " ") != "C-c Enter Escape PageUp F2 M-Enter S-Left C-Space KP0" {
		t.Fatalf("key call keys = %#v, want normalized keys", got.keys)
	}
}

func TestRouterKeysRejectsUnknownKey(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/keys foobar")); err != nil {
		t.Fatalf("HandleMessage(keys) error = %v", err)
	}
	if len(service.keyCalls) != 0 {
		t.Fatalf("keyCalls = %#v, want no send on invalid key", service.keyCalls)
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, `"foobar" is not a recognized tmux key name`) {
		t.Fatalf("last message = %q, want invalid key error", last.Text)
	}
	if !strings.Contains(last.Text, "PageUp") || !strings.Contains(last.Text, "M-Enter") || !strings.Contains(last.Text, "KP0-KP9") {
		t.Fatalf("last message = %q, want key usage examples", last.Text)
	}
}

func TestRouterKeysPromptUsesReply(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

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
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

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

func TestRouterEnterWithTextSendsTextAndEnter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	service.snapshotSequence = []snapshotResult{{body: "before"}, {body: "after\nmake test"}}
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouterWithPlainTextConfig(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "", PlainTextConfig{
		Echo:        plainTextEchoSnapshot,
		EchoLines:   8,
		EchoDelay:   time.Millisecond,
		EchoTimeout: 75 * time.Millisecond,
	})

	if err := router.HandleMessage(ctx, telegramMessage(7, 1, "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, telegramMessage(7, 2, "/enter make test")); err != nil {
		t.Fatalf("HandleMessage(enter with text) error = %v", err)
	}
	if len(service.sendCalls) != 1 {
		t.Fatalf("sendCalls = %#v, want one send", service.sendCalls)
	}
	if service.sendCalls[0].paneKey != "default:%5" || service.sendCalls[0].text != "make test" || !service.sendCalls[0].sendEnter {
		t.Fatalf("send call = %#v, want text send with Enter", service.sendCalls[0])
	}
	if len(service.keyCalls) != 0 {
		t.Fatalf("keyCalls = %#v, want no direct key send", service.keyCalls)
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if !strings.Contains(last.Text, "[default:%5]") || !strings.Contains(last.Text, "after\nmake test") {
		t.Fatalf("last message = %q, want execute snapshot", last.Text)
	}
}

func TestRouterNewCommandClearsPendingPrompt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

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
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
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
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

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

	text := formatPaneList([]tmuxconn.PaneRecord{{
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
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

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

func TestRouterWeixinSnapshotForcesTextMode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	service := newFakePaneService()
	messenger := &fakeMessenger{platform: "weixin"}
	replyBus := NewReplyBus(messenger, store, termrender.Options{})
	router := NewRouter(service, NewPaneRegistry(service), store, replyBus, NewFollowManager(service, replyBus, 20), 120, nil, "", "")

	if err := router.HandleMessage(ctx, weixinMessage("user@im.wechat", "1", "/select %5")); err != nil {
		t.Fatalf("HandleMessage(select) error = %v", err)
	}
	if err := router.HandleMessage(ctx, weixinMessage("user@im.wechat", "2", "/snapshot image")); err != nil {
		t.Fatalf("HandleMessage(snapshot image) error = %v", err)
	}

	messages := messenger.snapshot()
	last := messages[len(messages)-1]
	if len(last.Photo) != 0 {
		t.Fatalf("expected text snapshot for weixin, got photo message %#v", last)
	}
	if !strings.Contains(last.Text, "hello from pane") {
		t.Fatalf("snapshot text = %q, want pane text", last.Text)
	}
	if !strings.HasPrefix(last.Text, "```") {
		t.Fatalf("snapshot text = %q, want code block formatting", last.Text)
	}
}

func TestReplyBusReplySnapshotUsesConfiguredRenderOptions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenStore(ctx, filepath.Join(t.TempDir(), "tmuxconn.db"))
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
