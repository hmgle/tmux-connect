package daemon

import (
	"context"
	"fmt"
)

func (r *Router) handleFollow(ctx context.Context, message IncomingMessage, args string) error {
	mode, opts, err := parseFollowArgs(args)
	if err != nil {
		return r.replyFollowParseError(ctx, message, args)
	}
	switch mode {
	case "on":
		return r.enableFollow(ctx, message, opts)
	case "off":
		return r.disableFollow(ctx, message)
	default:
		return r.replyFollowUsage(ctx, message)
	}
}

func (r *Router) enableFollow(ctx context.Context, message IncomingMessage, opts FollowOptions) error {
	chat, paneKey, err := r.currentPaneForInbound(ctx, message, "command")
	if err != nil {
		return r.replyCurrentPaneError(ctx, message, paneKey, err)
	}
	if err := r.follow.EnableWithOptions(ctx, chat, paneKey, opts); err != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("follow failed: %v", err))
	}
	return r.replyFollowEnabled(ctx, chat, paneKey, r.follow.Options(chat.Key()))
}

func (r *Router) disableFollow(ctx context.Context, message IncomingMessage) error {
	chat := message.Chat
	paneKey := r.follow.CurrentPane(chat.Key())
	r.logInbound(ctx, message, paneKey, "")
	return r.replyFollowDisabled(ctx, chat, paneKey, r.follow.Disable(chat.Key()))
}
