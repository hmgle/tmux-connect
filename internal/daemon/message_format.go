package daemon

import (
	"html"
	"strings"

	"github.com/hmgle/tmux-connect/internal/telegram"
)

func decorateTelegramMessage(kind string, text string, opts telegram.SendOptions) (string, telegram.SendOptions) {
	if isPreformattedHTML(kind) {
		opts.ParseMode = telegram.ParseModeHTML
		return text, opts
	}
	if !usesTerminalHTML(kind) {
		return text, opts
	}
	opts.ParseMode = telegram.ParseModeHTML
	return renderTelegramTerminalHTML(text), opts
}

func isPreformattedHTML(kind string) bool {
	return kind == "panes"
}

func usesTerminalHTML(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "snapshot", "follow-initial", "follow-output":
		return true
	default:
		return false
	}
}

func renderTelegramTerminalHTML(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "<pre>(empty output)</pre>"
	}

	header, body, ok := strings.Cut(text, "\n")
	if !ok {
		return "<pre>" + html.EscapeString(text) + "</pre>"
	}

	header = strings.TrimSpace(header)
	body = strings.TrimSpace(body)
	if body == "" {
		return "<b>" + html.EscapeString(header) + "</b>"
	}
	return "<b>" + html.EscapeString(header) + "</b>\n<pre>" + html.EscapeString(body) + "</pre>"
}

func formatSnapshotCaption(paneKey string) string {
	paneKey = strings.TrimSpace(paneKey)
	if paneKey == "" {
		return "pane snapshot"
	}
	return paneKey + " snapshot"
}
