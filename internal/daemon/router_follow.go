package daemon

import "context"

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
