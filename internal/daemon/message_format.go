package daemon

import (
	"html"
	"strings"
)

func decorateTelegramMessage(kind string, text string, opts SendOptions) (string, SendOptions) {
	switch strings.TrimSpace(kind) {
	case "panes":
		opts.Format = MessageFormatTelegramHTML
		return renderTelegramPaneListHTML(text), opts
	}
	if !usesTerminalHTML(kind) {
		return text, opts
	}
	opts.Format = MessageFormatTelegramHTML
	return renderTelegramTerminalHTML(text), opts
}

func decorateSlackMessage(kind string, text string, opts SendOptions) (string, SendOptions) {
	return decorateCodeBlockMessage(kind, text, opts)
}

func decorateWhatsAppMessage(kind string, text string, opts SendOptions) (string, SendOptions) {
	return decorateCodeBlockMessage(kind, text, opts)
}

func decorateFeishuMessage(_ string, text string, opts SendOptions) (string, SendOptions) {
	return strings.TrimSpace(text), opts
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

func renderTelegramPaneListHTML(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "<pre>(empty output)</pre>"
	}

	lines := strings.Split(text, "\n")
	header := strings.TrimSpace(lines[0])
	bodyLines := lines[1:]
	footer := ""
	if len(bodyLines) > 0 {
		last := strings.TrimSpace(bodyLines[len(bodyLines)-1])
		if strings.HasPrefix(last, "Current: ") {
			footer = last
			bodyLines = bodyLines[:len(bodyLines)-1]
		}
	}

	var b strings.Builder
	if header != "" {
		b.WriteString("<b>")
		b.WriteString(html.EscapeString(header))
		b.WriteString("</b>")
	}
	if len(bodyLines) > 0 {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("<pre>")
		b.WriteString(html.EscapeString(strings.Join(bodyLines, "\n")))
		b.WriteString("</pre>")
	}
	if footer != "" {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(html.EscapeString(footer))
	}
	if b.Len() == 0 {
		return "<pre>" + html.EscapeString(text) + "</pre>"
	}
	return b.String()
}

func renderSlackCodeBlock(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "```(empty output)```"
	}
	return "```" + strings.ReplaceAll(text, "```", "`\u200b``") + "```"
}

const (
	discordEmbedColorSuccess = 0x2ECC71
	discordEmbedColorError   = 0xE74C3C
	discordEmbedColorInfo    = 0x3498DB
	discordMaxEmbedChars     = 4096
)

func decorateDiscordMessage(kind string, text string, opts SendOptions) (string, SendOptions) {
	switch strings.TrimSpace(kind) {
	case "panes", "snapshot", "follow-initial", "follow-output":
		text = strings.TrimSpace(text)
		if text == "" {
			text = "(empty output)"
		}
		text = truncateDiscordEmbedText(text, discordMaxEmbedChars-8)
		opts.Embed = &EmbedData{
			Description: "```\n" + text + "\n```",
			Color:       discordEmbedColorInfo,
		}
		return "", opts
	case "error", "unauthorized", "usage", "unknown-command":
		opts.Embed = &EmbedData{
			Title:       "Error",
			Description: truncateDiscordEmbedText(strings.TrimSpace(text), discordMaxEmbedChars),
			Color:       discordEmbedColorError,
		}
		return "", opts
	case "select", "clear", "unmanage", "send", "keys", "enter", "follow":
		opts.Embed = &EmbedData{
			Description: truncateDiscordEmbedText(strings.TrimSpace(text), discordMaxEmbedChars),
			Color:       discordEmbedColorSuccess,
		}
		return "", opts
	default:
		return text, opts
	}
}

func truncateDiscordEmbedText(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(text) <= limit {
		return text
	}
	if limit <= len("\n...") {
		return text[:limit]
	}
	return text[:limit-len("\n...")] + "\n..."
}

func formatSnapshotCaption(paneKey string) string {
	paneKey = strings.TrimSpace(paneKey)
	if paneKey == "" {
		return "pane snapshot"
	}
	return paneKey + " snapshot"
}
