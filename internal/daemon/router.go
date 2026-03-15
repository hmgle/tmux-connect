package daemon

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/portgle/tmux-connect/internal/tagb"
)

type IncomingMessage struct {
	ChatID    int64
	MessageID int64
	Text      string
	ChatType  string
}

type Router struct {
	service       paneService
	registry      *PaneRegistry
	store         *Store
	replyBus      *ReplyBus
	follow        *FollowManager
	snapshotLines int
	allowChats    map[int64]struct{}
}

func NewRouter(service paneService, registry *PaneRegistry, store *Store, replyBus *ReplyBus, follow *FollowManager, snapshotLines int, allowChats []int64) *Router {
	allowed := make(map[int64]struct{}, len(allowChats))
	for _, chatID := range allowChats {
		allowed[chatID] = struct{}{}
	}
	if snapshotLines <= 0 {
		snapshotLines = 120
	}
	return &Router{
		service:       service,
		registry:      registry,
		store:         store,
		replyBus:      replyBus,
		follow:        follow,
		snapshotLines: snapshotLines,
		allowChats:    allowed,
	}
}

func (r *Router) HandleMessage(ctx context.Context, message IncomingMessage) error {
	text := strings.TrimSpace(message.Text)
	if text == "" {
		return nil
	}

	currentPane, _ := r.store.CurrentPane(ctx, message.ChatID)
	r.replyBus.LogInbound(ctx, message.ChatID, currentPane, message.MessageID, "command", text)

	if !r.allowed(message.ChatID) {
		return r.replyBus.Reply(ctx, message.ChatID, "", "unauthorized", "chat is not allowed to use this bot")
	}

	command, args := parseCommand(text)
	switch command {
	case "/start", "/help":
		return r.replyBus.Reply(ctx, message.ChatID, currentPane, "help", helpText())
	case "/panes":
		return r.handlePanes(ctx, message.ChatID)
	case "/attach":
		return r.handleAttach(ctx, message.ChatID, args)
	case "/detach":
		return r.handleDetach(ctx, message.ChatID, args)
	case "/bind":
		return r.handleBind(ctx, message.ChatID, args)
	case "/current":
		return r.handleCurrent(ctx, message.ChatID)
	case "/snapshot":
		return r.handleSnapshot(ctx, message.ChatID, args)
	case "/send":
		return r.handleSend(ctx, message.ChatID, args)
	case "/enter":
		return r.handleEnter(ctx, message.ChatID)
	case "/ctrlc", "/ctrl-c":
		return r.handleCtrlC(ctx, message.ChatID)
	case "/follow":
		return r.handleFollow(ctx, message.ChatID, args)
	default:
		return r.replyBus.Reply(ctx, message.ChatID, currentPane, "unknown-command", "unknown command\n\n"+helpText())
	}
}

func (r *Router) handlePanes(ctx context.Context, chatID int64) error {
	if err := r.registry.Refresh(ctx); err != nil {
		return r.replyBus.Reply(ctx, chatID, "", "error", fmt.Sprintf("list panes failed: %v", err))
	}
	bindings, err := r.store.ListBindings(ctx, chatID)
	if err != nil {
		return err
	}
	current, err := r.store.CurrentPane(ctx, chatID)
	if err != nil {
		return err
	}

	boundSet := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		boundSet[binding] = struct{}{}
	}

	var lines []string
	lines = append(lines, "Panes:")
	for _, record := range r.registry.All() {
		key := record.Info.Target.PaneKey()
		flags := make([]string, 0, 3)
		if record.Metadata.Managed {
			flags = append(flags, "managed")
		} else {
			flags = append(flags, "unmanaged")
		}
		if _, ok := boundSet[key]; ok {
			flags = append(flags, "bound")
		}
		if key == current {
			flags = append(flags, "current")
		}
		label := strings.TrimSpace(record.Metadata.Label)
		if label == "" {
			label = record.Info.CurrentCmd
		}
		lines = append(lines, fmt.Sprintf("- %s [%s] %s (%s/%s)", key, strings.Join(flags, ","), label, record.Info.SessionName, record.Info.WindowName))
	}
	if current == "" {
		lines = append(lines, "", "Current: none")
	} else {
		lines = append(lines, "", "Current: "+current)
	}
	lines = append(lines, "Follow: "+onOff(r.follow.IsEnabled(chatID)))
	return r.replyBus.Reply(ctx, chatID, current, "panes", strings.Join(lines, "\n"))
}

func (r *Router) handleAttach(ctx context.Context, chatID int64, args string) error {
	ref := strings.TrimSpace(args)
	if ref == "" {
		return r.replyBus.Reply(ctx, chatID, "", "usage", "usage: /attach <pane>")
	}
	record, err := r.service.Attach(ctx, ref, "unknown", "")
	if err != nil {
		return r.replyBus.Reply(ctx, chatID, "", "error", fmt.Sprintf("attach failed: %v", err))
	}
	_ = r.registry.Refresh(ctx)
	return r.replyBus.Reply(ctx, chatID, record.Info.Target.PaneKey(), "attach", fmt.Sprintf("attached %s", record.Info.Target.PaneKey()))
}

func (r *Router) handleDetach(ctx context.Context, chatID int64, args string) error {
	ref := strings.TrimSpace(args)
	if ref == "" {
		return r.replyBus.Reply(ctx, chatID, "", "usage", "usage: /detach <pane>")
	}
	record, err := r.service.Inspect(ctx, ref)
	if err != nil {
		return r.replyBus.Reply(ctx, chatID, "", "error", fmt.Sprintf("inspect failed: %v", err))
	}
	paneKey := record.Info.Target.PaneKey()
	if err := r.service.Detach(ctx, ref); err != nil {
		return r.replyBus.Reply(ctx, chatID, paneKey, "error", fmt.Sprintf("detach failed: %v", err))
	}
	_ = r.store.UnbindPaneEverywhere(ctx, paneKey)
	r.follow.StopPane(paneKey)
	_ = r.registry.Refresh(ctx)
	return r.replyBus.Reply(ctx, chatID, paneKey, "detach", fmt.Sprintf("detached %s", paneKey))
}

func (r *Router) handleBind(ctx context.Context, chatID int64, args string) error {
	ref := strings.TrimSpace(args)
	if ref == "" {
		return r.replyBus.Reply(ctx, chatID, "", "usage", "usage: /bind <pane>")
	}
	record, err := r.service.Inspect(ctx, ref)
	if err != nil {
		return r.replyBus.Reply(ctx, chatID, "", "error", fmt.Sprintf("inspect failed: %v", err))
	}
	if !record.Metadata.Managed {
		return r.replyBus.Reply(ctx, chatID, record.Info.Target.PaneKey(), "error", "pane is not managed; run /attach first")
	}
	paneKey := record.Info.Target.PaneKey()
	if err := r.store.BindPane(ctx, chatID, paneKey); err != nil {
		return err
	}
	if err := r.store.SetCurrentPane(ctx, chatID, paneKey); err != nil {
		return err
	}
	if r.follow.IsEnabled(chatID) {
		if err := r.follow.Enable(ctx, chatID, paneKey); err != nil {
			return r.replyBus.Reply(ctx, chatID, paneKey, "error", fmt.Sprintf("follow switch failed: %v", err))
		}
	}
	return r.replyBus.Reply(ctx, chatID, paneKey, "bind", fmt.Sprintf("bound current chat to %s", paneKey))
}

func (r *Router) handleCurrent(ctx context.Context, chatID int64) error {
	current, err := r.store.CurrentPane(ctx, chatID)
	if err != nil {
		return err
	}
	if current == "" {
		return r.replyBus.Reply(ctx, chatID, "", "current", "no pane is currently bound")
	}
	record, err := r.service.Inspect(ctx, current)
	if err != nil {
		_ = r.store.SetCurrentPane(ctx, chatID, "")
		return r.replyBus.Reply(ctx, chatID, current, "error", fmt.Sprintf("current pane is unavailable: %v", err))
	}
	return r.replyBus.Reply(ctx, chatID, current, "current", formatCurrent(record, r.follow.IsEnabled(chatID)))
}

func (r *Router) handleSnapshot(ctx context.Context, chatID int64, args string) error {
	paneKey, err := r.requireCurrentPane(ctx, chatID)
	if err != nil {
		return r.replyBus.Reply(ctx, chatID, "", "error", err.Error())
	}
	lines, err := optionalInt(args, r.snapshotLines)
	if err != nil {
		return r.replyBus.Reply(ctx, chatID, paneKey, "usage", "usage: /snapshot [lines]")
	}
	body, err := r.service.Snapshot(ctx, paneKey, lines)
	if err != nil {
		return r.replyBus.Reply(ctx, chatID, paneKey, "error", fmt.Sprintf("snapshot failed: %v", err))
	}
	return r.replyBus.Reply(ctx, chatID, paneKey, "snapshot", formatFollowMessage(paneKey, body, 3500))
}

func (r *Router) handleSend(ctx context.Context, chatID int64, args string) error {
	paneKey, err := r.requireCurrentPane(ctx, chatID)
	if err != nil {
		return r.replyBus.Reply(ctx, chatID, "", "error", err.Error())
	}
	text := strings.TrimSpace(args)
	if text == "" {
		return r.replyBus.Reply(ctx, chatID, paneKey, "usage", "usage: /send <text>")
	}
	if err := r.service.Send(ctx, paneKey, text, false); err != nil {
		return r.replyBus.Reply(ctx, chatID, paneKey, "error", fmt.Sprintf("send failed: %v", err))
	}
	return r.replyBus.Reply(ctx, chatID, paneKey, "send", fmt.Sprintf("sent to %s", paneKey))
}

func (r *Router) handleEnter(ctx context.Context, chatID int64) error {
	paneKey, err := r.requireCurrentPane(ctx, chatID)
	if err != nil {
		return r.replyBus.Reply(ctx, chatID, "", "error", err.Error())
	}
	if err := r.service.Enter(ctx, paneKey); err != nil {
		return r.replyBus.Reply(ctx, chatID, paneKey, "error", fmt.Sprintf("enter failed: %v", err))
	}
	return r.replyBus.Reply(ctx, chatID, paneKey, "enter", fmt.Sprintf("sent Enter to %s", paneKey))
}

func (r *Router) handleCtrlC(ctx context.Context, chatID int64) error {
	paneKey, err := r.requireCurrentPane(ctx, chatID)
	if err != nil {
		return r.replyBus.Reply(ctx, chatID, "", "error", err.Error())
	}
	if err := r.service.CtrlC(ctx, paneKey); err != nil {
		return r.replyBus.Reply(ctx, chatID, paneKey, "error", fmt.Sprintf("ctrl-c failed: %v", err))
	}
	return r.replyBus.Reply(ctx, chatID, paneKey, "ctrl-c", fmt.Sprintf("sent Ctrl-C to %s", paneKey))
}

func (r *Router) handleFollow(ctx context.Context, chatID int64, args string) error {
	mode := strings.ToLower(strings.TrimSpace(args))
	switch mode {
	case "on":
		paneKey, err := r.requireCurrentPane(ctx, chatID)
		if err != nil {
			return r.replyBus.Reply(ctx, chatID, "", "error", err.Error())
		}
		if err := r.follow.Enable(ctx, chatID, paneKey); err != nil {
			return r.replyBus.Reply(ctx, chatID, paneKey, "error", fmt.Sprintf("follow failed: %v", err))
		}
		return r.replyBus.Reply(ctx, chatID, paneKey, "follow", fmt.Sprintf("follow enabled for %s", paneKey))
	case "off":
		if !r.follow.Disable(chatID) {
			return r.replyBus.Reply(ctx, chatID, "", "follow", "follow is already off")
		}
		return r.replyBus.Reply(ctx, chatID, "", "follow", "follow disabled")
	default:
		return r.replyBus.Reply(ctx, chatID, "", "usage", "usage: /follow on|off")
	}
}

func (r *Router) requireCurrentPane(ctx context.Context, chatID int64) (string, error) {
	current, err := r.store.CurrentPane(ctx, chatID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(current) == "" {
		return "", fmt.Errorf("no current pane; run /bind <pane> first")
	}
	record, err := r.service.Inspect(ctx, current)
	if err != nil {
		_ = r.store.SetCurrentPane(ctx, chatID, "")
		return "", fmt.Errorf("current pane is unavailable: %w", err)
	}
	if !record.Metadata.Managed {
		_ = r.store.SetCurrentPane(ctx, chatID, "")
		return "", fmt.Errorf("current pane is no longer managed")
	}
	return record.Info.Target.PaneKey(), nil
}

func (r *Router) allowed(chatID int64) bool {
	if len(r.allowChats) == 0 {
		return true
	}
	_, ok := r.allowChats[chatID]
	return ok
}

func parseCommand(text string) (string, string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ""
	}
	if !strings.HasPrefix(text, "/") {
		return "", text
	}
	command := text
	args := ""
	if idx := strings.IndexAny(text, " \n\t"); idx >= 0 {
		command = text[:idx]
		args = strings.TrimSpace(text[idx+1:])
	}
	if mention := strings.Index(command, "@"); mention >= 0 {
		command = command[:mention]
	}
	return strings.ToLower(command), args
}

func optionalInt(value string, fallback int) (int, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func formatCurrent(record tagb.PaneRecord, following bool) string {
	lines := []string{
		"Current pane: " + record.Info.Target.PaneKey(),
		"Session: " + record.Info.SessionName,
		"Window: " + record.Info.WindowName,
		"Command: " + record.Info.CurrentCmd,
		"Managed: " + onOff(record.Metadata.Managed),
		"Agent: " + string(record.Metadata.Agent),
		"Label: " + strings.TrimSpace(record.Metadata.Label),
		"Follow: " + onOff(following),
	}
	return strings.Join(lines, "\n")
}

func helpText() string {
	return strings.Join([]string{
		"Commands:",
		"/panes",
		"/attach <pane>",
		"/detach <pane>",
		"/bind <pane>",
		"/current",
		"/snapshot [lines]",
		"/send <text>",
		"/enter",
		"/ctrlc",
		"/follow on|off",
	}, "\n")
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}
