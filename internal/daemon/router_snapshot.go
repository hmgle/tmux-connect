package daemon

import "context"

func (r *Router) handleSnapshot(ctx context.Context, message IncomingMessage, args string) error {
	chat, paneKey, err := r.currentPaneForInbound(ctx, message, "command")
	if err != nil {
		return r.replyCurrentPaneError(ctx, message, paneKey, err)
	}
	lines, mode, err := parseSnapshotArgs(args, r.snapshotLines)
	if err != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "usage", "usage: "+formatCommandUsage(r.commandPrefix(chat), snapshotCommandUsage(chat.Platform)))
	}
	return r.replySnapshotForMode(ctx, chat, paneKey, lines, mode)
}
