package daemon

import "strings"

func threadReplyTarget(opts SendOptions) string {
	if threadID := strings.TrimSpace(opts.ThreadID); threadID != "" {
		return threadID
	}
	return strings.TrimSpace(opts.ReplyToMessageID)
}

func decorateCodeBlockMessage(kind string, text string, opts SendOptions) (string, SendOptions) {
	switch strings.TrimSpace(kind) {
	case "panes", "snapshot", "follow-initial", "follow-output":
		return renderSlackCodeBlock(text), opts
	default:
		return text, opts
	}
}
