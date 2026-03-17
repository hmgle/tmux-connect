package daemon

import (
	"context"
	"fmt"
	"html"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/portgle/tmux-connect/internal/tagb"
	"github.com/portgle/tmux-connect/internal/tmux"
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

	if !r.allowed(message.ChatID) {
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, message.ChatID, "", "unauthorized", "chat is not allowed to use this bot")
	}

	command, args := parseCommand(text)
	switch command {
	case "/start", "/help":
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, message.ChatID, "", "help", helpText())
	case "/panes":
		return r.handlePanes(ctx, message)
	case "/select":
		return r.handleSelect(ctx, message, args)
	case "/clear":
		return r.handleClear(ctx, message)
	case "/unmanage":
		return r.handleUnmanage(ctx, message, args)
	case "/current":
		return r.handleCurrent(ctx, message)
	case "/snapshot":
		return r.handleSnapshot(ctx, message, args)
	case "/send":
		return r.handleSend(ctx, message, args)
	case "/enter":
		return r.handleEnter(ctx, message)
	case "/ctrlc", "/ctrl-c":
		return r.handleCtrlC(ctx, message)
	case "/follow":
		return r.handleFollow(ctx, message, args)
	default:
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, message.ChatID, "", "unknown-command", "unknown command\n\n"+helpText())
	}
}

func (r *Router) handlePanes(ctx context.Context, message IncomingMessage) error {
	chatID := message.ChatID
	r.logInbound(ctx, message, "", "")
	if err := r.registry.Refresh(ctx); err != nil {
		return r.replyBus.Reply(ctx, chatID, "", "error", fmt.Sprintf("list panes failed: %v", err))
	}
	current, err := r.store.CurrentPane(ctx, chatID)
	if err != nil {
		return err
	}
	return r.replyBus.Reply(ctx, chatID, "", "panes", formatPaneList(r.registry.All(), current, r.follow.IsEnabled(chatID)))
}

func (r *Router) handleUnmanage(ctx context.Context, message IncomingMessage, args string) error {
	chatID := message.ChatID
	r.logInbound(ctx, message, "", "")
	ref := strings.TrimSpace(args)
	if ref == "" {
		return r.replyBus.Reply(ctx, chatID, "", "usage", "usage: /unmanage <pane>")
	}
	record, err := r.service.Inspect(ctx, ref)
	if err != nil {
		return r.replyBus.Reply(ctx, chatID, "", "error", fmt.Sprintf("inspect failed: %v", err))
	}
	paneKey := record.Info.Target.PaneKey()
	if err := r.service.Detach(ctx, ref); err != nil {
		return r.replyBus.Reply(ctx, chatID, "", "error", fmt.Sprintf("unmanage failed: %v", err))
	}
	var cleanupErrs []string
	if err := r.store.UnbindPaneEverywhere(ctx, paneKey); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Sprintf("failed to clear local bindings: %v", err))
	}
	r.follow.StopPane(paneKey)
	r.registry.MarkDirty()
	if len(cleanupErrs) > 0 {
		return r.replyBus.Reply(ctx, chatID, paneKey, "error", fmt.Sprintf("unmanaged %s but cleanup was incomplete: %s", paneKey, strings.Join(cleanupErrs, "; ")))
	}
	return r.replyBus.Reply(ctx, chatID, "", "unmanage", fmt.Sprintf("unmanaged %s", paneKey))
}

func (r *Router) handleSelect(ctx context.Context, message IncomingMessage, args string) error {
	chatID := message.ChatID
	ref := strings.TrimSpace(args)
	if ref == "" {
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, chatID, "", "usage", "usage: /select <pane>")
	}
	record, err := r.service.Inspect(ctx, ref)
	if err != nil {
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, chatID, "", "error", fmt.Sprintf("inspect failed: %v", err))
	}
	if !record.Metadata.Managed {
		record, err = r.service.Attach(ctx, ref, "unknown", "")
		if err != nil {
			r.logInbound(ctx, message, "", "")
			return r.replyBus.Reply(ctx, chatID, "", "error", fmt.Sprintf("select failed: %v", err))
		}
		r.registry.MarkDirty()
	}
	paneKey := record.Info.Target.PaneKey()
	if err := r.store.BindPane(ctx, chatID, paneKey); err != nil {
		return err
	}
	if err := r.store.SetCurrentPane(ctx, chatID, paneKey); err != nil {
		return err
	}
	r.logInbound(ctx, message, paneKey, string(record.Metadata.Agent))
	if r.follow.IsEnabled(chatID) {
		if err := r.follow.EnableWithOptions(ctx, chatID, paneKey, r.follow.Options(chatID)); err != nil {
			return r.replyBus.Reply(ctx, chatID, paneKey, "error", fmt.Sprintf("follow switch failed: %v", err))
		}
	}
	return r.replyBus.Reply(ctx, chatID, paneKey, "select", fmt.Sprintf("selected %s", paneKey))
}

func (r *Router) handleClear(ctx context.Context, message IncomingMessage) error {
	chatID := message.ChatID
	current, err := r.store.CurrentPane(ctx, chatID)
	if err != nil {
		return err
	}
	if current == "" {
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, chatID, "", "clear", "no pane is currently selected")
	}
	if err := r.store.SetCurrentPane(ctx, chatID, ""); err != nil {
		return err
	}
	r.follow.Disable(chatID)
	r.logInbound(ctx, message, current, "")
	return r.replyBus.Reply(ctx, chatID, current, "clear", "cleared current pane")
}

func (r *Router) handleCurrent(ctx context.Context, message IncomingMessage) error {
	chatID := message.ChatID
	current, err := r.store.CurrentPane(ctx, chatID)
	if err != nil {
		return err
	}
	if current == "" {
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, chatID, "", "current", "no pane is currently selected")
	}
	r.logInbound(ctx, message, current, "")
	record, err := r.service.Inspect(ctx, current)
	if err != nil {
		_ = r.store.SetCurrentPane(ctx, chatID, "")
		return r.replyBus.Reply(ctx, chatID, current, "error", fmt.Sprintf("current pane is unavailable: %v", err))
	}
	return r.replyBus.Reply(ctx, chatID, current, "current", formatCurrent(record, r.follow.IsEnabled(chatID)))
}

func (r *Router) handleSnapshot(ctx context.Context, message IncomingMessage, args string) error {
	chatID := message.ChatID
	paneKey, err := r.requireCurrentPane(ctx, chatID)
	if err != nil {
		r.logInbound(ctx, message, paneKey, "")
		return r.replyBus.Reply(ctx, chatID, paneKey, "error", err.Error())
	}
	r.logInbound(ctx, message, paneKey, "")
	lines, mode, err := parseSnapshotArgs(args, r.snapshotLines)
	if err != nil {
		return r.replyBus.Reply(ctx, chatID, paneKey, "usage", "usage: /snapshot [lines] [image|text]")
	}
	body, err := r.service.Snapshot(ctx, paneKey, lines)
	if err != nil {
		return r.replyBus.Reply(ctx, chatID, paneKey, "error", fmt.Sprintf("snapshot failed: %v", err))
	}
	if mode == snapshotModeText {
		return r.replyBus.Reply(ctx, chatID, paneKey, "snapshot", formatFollowMessage(paneKey, body, 3500))
	}
	richBody, richErr := r.service.SnapshotRich(ctx, paneKey, lines)
	if richErr == nil && strings.TrimSpace(richBody) != "" {
		return r.replyBus.ReplySnapshot(ctx, chatID, paneKey, body, richBody)
	}
	return r.replyBus.Reply(ctx, chatID, paneKey, "snapshot", formatFollowMessage(paneKey, body, 3500))
}

func (r *Router) handleSend(ctx context.Context, message IncomingMessage, args string) error {
	chatID := message.ChatID
	paneKey, err := r.requireCurrentPane(ctx, chatID)
	if err != nil {
		r.logInbound(ctx, message, paneKey, "")
		return r.replyBus.Reply(ctx, chatID, paneKey, "error", err.Error())
	}
	r.logInbound(ctx, message, paneKey, "")
	text := strings.TrimSpace(args)
	if text == "" {
		return r.replyBus.Reply(ctx, chatID, paneKey, "usage", "usage: /send <text>")
	}
	if err := r.service.SendManaged(ctx, paneKey, text, false); err != nil {
		return r.replyBus.Reply(ctx, chatID, paneKey, "error", fmt.Sprintf("send failed: %v", err))
	}
	return r.replyBus.Reply(ctx, chatID, paneKey, "send", fmt.Sprintf("sent to %s", paneKey))
}

func (r *Router) handleEnter(ctx context.Context, message IncomingMessage) error {
	chatID := message.ChatID
	paneKey, err := r.requireCurrentPane(ctx, chatID)
	if err != nil {
		r.logInbound(ctx, message, paneKey, "")
		return r.replyBus.Reply(ctx, chatID, paneKey, "error", err.Error())
	}
	r.logInbound(ctx, message, paneKey, "")
	if err := r.service.EnterManaged(ctx, paneKey); err != nil {
		return r.replyBus.Reply(ctx, chatID, paneKey, "error", fmt.Sprintf("enter failed: %v", err))
	}
	return r.replyBus.Reply(ctx, chatID, paneKey, "enter", fmt.Sprintf("sent Enter to %s", paneKey))
}

func (r *Router) handleCtrlC(ctx context.Context, message IncomingMessage) error {
	chatID := message.ChatID
	paneKey, err := r.requireCurrentPane(ctx, chatID)
	if err != nil {
		r.logInbound(ctx, message, paneKey, "")
		return r.replyBus.Reply(ctx, chatID, paneKey, "error", err.Error())
	}
	r.logInbound(ctx, message, paneKey, "")
	if err := r.service.CtrlCManaged(ctx, paneKey); err != nil {
		return r.replyBus.Reply(ctx, chatID, paneKey, "error", fmt.Sprintf("ctrl-c failed: %v", err))
	}
	return r.replyBus.Reply(ctx, chatID, paneKey, "ctrl-c", fmt.Sprintf("sent Ctrl-C to %s", paneKey))
}

func (r *Router) handleFollow(ctx context.Context, message IncomingMessage, args string) error {
	chatID := message.ChatID
	mode, opts, err := parseFollowArgs(args)
	if err != nil {
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, chatID, "", "usage", "usage: /follow on [interval]|off")
	}
	switch mode {
	case "on":
		paneKey, err := r.requireCurrentPane(ctx, chatID)
		if err != nil {
			r.logInbound(ctx, message, paneKey, "")
			return r.replyBus.Reply(ctx, chatID, paneKey, "error", err.Error())
		}
		r.logInbound(ctx, message, paneKey, "")
		if err := r.follow.EnableWithOptions(ctx, chatID, paneKey, opts); err != nil {
			return r.replyBus.Reply(ctx, chatID, paneKey, "error", fmt.Sprintf("follow failed: %v", err))
		}
		resolved := r.follow.Options(chatID)
		return r.replyBus.Reply(ctx, chatID, paneKey, "follow", fmt.Sprintf("follow enabled for %s (min interval %s)", paneKey, resolved.MinInterval))
	case "off":
		paneKey := r.follow.CurrentPane(chatID)
		r.logInbound(ctx, message, paneKey, "")
		if !r.follow.Disable(chatID) {
			return r.replyBus.Reply(ctx, chatID, paneKey, "follow", "follow is already off")
		}
		return r.replyBus.Reply(ctx, chatID, paneKey, "follow", "follow disabled")
	default:
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, chatID, "", "usage", "usage: /follow on [interval]|off")
	}
}

func (r *Router) requireCurrentPane(ctx context.Context, chatID int64) (string, error) {
	current, err := r.store.CurrentPane(ctx, chatID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(current) == "" {
		return "", fmt.Errorf("no current pane; run /select <pane> first")
	}
	record, err := r.service.Inspect(ctx, current)
	if err != nil {
		_ = r.store.SetCurrentPane(ctx, chatID, "")
		return current, fmt.Errorf("current pane is unavailable: %w", err)
	}
	if !record.Metadata.Managed {
		_ = r.store.SetCurrentPane(ctx, chatID, "")
		return current, fmt.Errorf("current pane is no longer managed")
	}
	return record.Info.Target.PaneKey(), nil
}

func (r *Router) logInbound(ctx context.Context, message IncomingMessage, paneKey string, agent string) {
	r.replyBus.LogInbound(ctx, message.ChatID, paneKey, agent, message.MessageID, "command", message.Text)
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

func parseFollowArgs(value string) (string, FollowOptions, error) {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return "", FollowOptions{}, fmt.Errorf("missing follow mode")
	}

	mode := strings.ToLower(fields[0])
	switch mode {
	case "off":
		if len(fields) != 1 {
			return "", FollowOptions{}, fmt.Errorf("unexpected follow args")
		}
		return mode, FollowOptions{}, nil
	case "on":
		if len(fields) == 1 {
			return mode, FollowOptions{}, nil
		}
		if len(fields) != 2 {
			return "", FollowOptions{}, fmt.Errorf("unexpected follow args")
		}
		interval, err := parsePositiveDuration(fields[1])
		if err != nil {
			return "", FollowOptions{}, err
		}
		return mode, FollowOptions{MinInterval: interval}, nil
	default:
		return "", FollowOptions{}, fmt.Errorf("unknown follow mode")
	}
}

func parsePositiveDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("duration is required")
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0, fmt.Errorf("duration must be > 0")
		}
		return time.Duration(seconds) * time.Second, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("duration must be > 0")
	}
	return parsed, nil
}

type snapshotMode string

const (
	snapshotModeImage snapshotMode = "image"
	snapshotModeText  snapshotMode = "text"
)

func parseSnapshotArgs(value string, fallbackLines int) (int, snapshotMode, error) {
	lines := fallbackLines
	mode := snapshotModeImage
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return lines, mode, nil
	}

	for _, field := range fields {
		switch strings.ToLower(strings.TrimSpace(field)) {
		case "":
			continue
		case string(snapshotModeImage):
			mode = snapshotModeImage
		case string(snapshotModeText):
			mode = snapshotModeText
		default:
			parsed, err := strconv.Atoi(field)
			if err != nil {
				return 0, "", fmt.Errorf("invalid snapshot arg %q", field)
			}
			lines = parsed
		}
	}

	if lines <= 0 {
		return 0, "", fmt.Errorf("snapshot lines must be > 0")
	}
	return lines, mode, nil
}

func formatCurrent(record tagb.PaneRecord, following bool) string {
	lines := []string{
		"Current pane: " + displayPaneKey(record.Info.Target.PaneKey()),
		"Where: " + formatPaneWhere(record.Info),
		"Command: " + displayValue(record.Info.CurrentCmd),
		"Dir: " + formatPaneDir(record.Info.CurrentPath),
		"Follow: " + onOff(following),
	}
	return strings.Join(lines, "\n")
}

func formatPaneList(records []tagb.PaneRecord, current string, following bool) string {
	if len(records) == 0 {
		return "No managed panes.\n\nCurrent: none · Follow: " + onOff(following)
	}

	type row struct {
		marker string
		pane   string
		cmd    string
		dir    string
		where  string
	}

	rows := make([]row, len(records))
	header := row{marker: " ", pane: "Pane", cmd: "Cmd", dir: "Dir", where: "Where"}
	wPane := utf8.RuneCountInString(header.pane)
	wCmd := utf8.RuneCountInString(header.cmd)
	wDir := utf8.RuneCountInString(header.dir)

	for i, rec := range records {
		r := row{
			marker: " ",
			pane:   displayPaneKey(rec.Info.Target.PaneKey()),
			cmd:    shortenDisplay(displayValue(rec.Info.CurrentCmd), 16),
			dir:    formatPaneDir(rec.Info.CurrentPath),
			where:  shortenDisplay(formatPaneWhere(rec.Info), 20),
		}
		if rec.Info.Target.PaneKey() == current {
			r.marker = ">"
		}
		rows[i] = r
		if n := utf8.RuneCountInString(r.pane); n > wPane {
			wPane = n
		}
		if n := utf8.RuneCountInString(r.cmd); n > wCmd {
			wCmd = n
		}
		if n := utf8.RuneCountInString(r.dir); n > wDir {
			wDir = n
		}
	}

	var b strings.Builder
	b.WriteString("<b>Panes:</b>\n<pre>")
	writeRow := func(r row) {
		b.WriteString(r.marker)
		b.WriteByte(' ')
		writePadded(&b, r.pane, wPane)
		b.WriteString("  ")
		writePadded(&b, r.cmd, wCmd)
		b.WriteString("  ")
		writePadded(&b, r.dir, wDir)
		b.WriteString("  ")
		b.WriteString(html.EscapeString(r.where))
	}
	writeRow(header)
	for _, r := range rows {
		b.WriteByte('\n')
		writeRow(r)
	}
	b.WriteString("</pre>\nCurrent: ")
	b.WriteString(html.EscapeString(displayCurrent(current)))
	b.WriteString(" · Follow: ")
	b.WriteString(onOff(following))
	return b.String()
}

func writePadded(b *strings.Builder, s string, width int) {
	b.WriteString(html.EscapeString(s))
	for i := utf8.RuneCountInString(s); i < width; i++ {
		b.WriteByte(' ')
	}
}

func displayCurrent(current string) string {
	current = strings.TrimSpace(current)
	if current == "" {
		return "none"
	}
	return displayPaneKey(current)
}

func displayPaneKey(paneKey string) string {
	paneKey = strings.TrimSpace(paneKey)
	if paneKey == "" {
		return "-"
	}
	if strings.HasPrefix(paneKey, "default:") {
		return strings.TrimPrefix(paneKey, "default:")
	}
	return paneKey
}

func formatPaneDir(currentPath string) string {
	currentPath = strings.TrimSpace(currentPath)
	if currentPath == "" {
		return "-"
	}
	dirName := filepath.Base(filepath.Clean(currentPath))
	if strings.TrimSpace(dirName) == "" {
		dirName = currentPath
	}
	return shortenDisplay(dirName, 14)
}

func formatPaneWhere(info tmux.PaneInfo) string {
	return strings.TrimSpace(displayValue(info.SessionName) + "/" + displayValue(info.WindowName))
}

func displayValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func shortenDisplay(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	runes := []rune(value)
	if len(runes) <= maxRunes || maxRunes <= 0 {
		return value
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	remaining := maxRunes - 3
	prefix := remaining / 2
	suffix := remaining - prefix
	if prefix == 0 {
		return "..." + string(runes[len(runes)-suffix:])
	}
	if suffix == 0 {
		return string(runes[:prefix]) + "..."
	}
	return string(runes[:prefix]) + "..." + string(runes[len(runes)-suffix:])
}

func helpText() string {
	return strings.Join([]string{
		"Commands:",
		"/panes",
		"/select <pane>",
		"/clear",
		"/unmanage <pane>",
		"/current",
		"/snapshot [lines] [image|text]",
		"/send <text>",
		"/enter",
		"/ctrlc",
		"/follow on [interval]|off",
	}, "\n")
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}
