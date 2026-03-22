package daemon

import "context"

type routerCommandHandler func(*Router, context.Context, IncomingMessage, string) error

var routerCommandHandlers = map[string]routerCommandHandler{
	"start":    dispatchHelpCommand,
	"help":     dispatchHelpCommand,
	"panes":    dispatchPanesCommand,
	"select":   dispatchSelectCommand,
	"clear":    dispatchClearCommand,
	"unmanage": dispatchUnmanageCommand,
	"current":  dispatchCurrentCommand,
	"snapshot": dispatchSnapshotCommand,
	"send":     dispatchSendCommand,
	"keys":     dispatchKeysCommand,
	"enter":    dispatchEnterCommand,
	"ctrlc":    dispatchCtrlCCommand,
	"follow":   dispatchFollowCommand,
}

func (r *Router) dispatchCommand(ctx context.Context, message IncomingMessage, command string, args string) error {
	handler, ok := routerCommandHandlers[command]
	if !ok {
		r.logInbound(ctx, message, "", "")
		return r.replyBus.Reply(ctx, message.Chat, "", "unknown-command", "unknown command\n\n"+r.helpText(message.Chat))
	}
	return handler(r, ctx, message, args)
}

func dispatchHelpCommand(r *Router, ctx context.Context, message IncomingMessage, _ string) error {
	r.logInbound(ctx, message, "", "")
	return r.replyBus.Reply(ctx, message.Chat, "", "help", r.helpText(message.Chat))
}

func dispatchPanesCommand(r *Router, ctx context.Context, message IncomingMessage, _ string) error {
	return r.handlePanes(ctx, message)
}

func dispatchSelectCommand(r *Router, ctx context.Context, message IncomingMessage, args string) error {
	return r.handleSelect(ctx, message, args)
}

func dispatchClearCommand(r *Router, ctx context.Context, message IncomingMessage, _ string) error {
	return r.handleClear(ctx, message)
}

func dispatchUnmanageCommand(r *Router, ctx context.Context, message IncomingMessage, args string) error {
	return r.handleUnmanage(ctx, message, args)
}

func dispatchCurrentCommand(r *Router, ctx context.Context, message IncomingMessage, _ string) error {
	return r.handleCurrent(ctx, message)
}

func dispatchSnapshotCommand(r *Router, ctx context.Context, message IncomingMessage, args string) error {
	return r.handleSnapshot(ctx, message, args)
}

func dispatchSendCommand(r *Router, ctx context.Context, message IncomingMessage, args string) error {
	return r.handleSend(ctx, message, args)
}

func dispatchKeysCommand(r *Router, ctx context.Context, message IncomingMessage, args string) error {
	return r.handleKeys(ctx, message, args)
}

func dispatchEnterCommand(r *Router, ctx context.Context, message IncomingMessage, args string) error {
	return r.handleEnter(ctx, message, args)
}

func dispatchCtrlCCommand(r *Router, ctx context.Context, message IncomingMessage, _ string) error {
	return r.handleCtrlC(ctx, message)
}

func dispatchFollowCommand(r *Router, ctx context.Context, message IncomingMessage, args string) error {
	return r.handleFollow(ctx, message, args)
}
