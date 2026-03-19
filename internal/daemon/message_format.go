package daemon

import (
	"html"
	"strings"
)

func decorateTelegramMessage(kind string, text string, opts SendOptions) (string, SendOptions) {
	if isPreformattedHTML(kind) {
		opts.Format = MessageFormatTelegramHTML
		return text, opts
	}
	if !usesTerminalHTML(kind) {
		return text, opts
	}
	opts.Format = MessageFormatTelegramHTML
	return renderTelegramTerminalHTML(text), opts
}

func decorateSlackMessage(kind string, text string, opts SendOptions) (string, SendOptions) {
	switch strings.TrimSpace(kind) {
	case "panes", "snapshot", "follow-initial", "follow-output":
		return renderSlackCodeBlock(text), opts
	default:
		return text, opts
	}
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

func renderSlackCodeBlock(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "```(empty output)```"
	}
	return "```" + strings.ReplaceAll(text, "```", "`\u200b``") + "```"
}

func formatSnapshotCaption(paneKey string) string {
	paneKey = strings.TrimSpace(paneKey)
	if paneKey == "" {
		return "pane snapshot"
	}
	return paneKey + " snapshot"
}
