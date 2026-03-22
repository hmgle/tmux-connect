package daemon

import "context"

func (r *Router) logInbound(ctx context.Context, message IncomingMessage, paneKey string, agent string) {
	r.logInboundKind(ctx, message, paneKey, agent, "command")
}

func (r *Router) logInboundKind(ctx context.Context, message IncomingMessage, paneKey string, agent string, kind string) {
	r.replyBus.LogInbound(ctx, message, paneKey, agent, kind)
}

func (r *Router) allowed(chat ChatRef) bool {
	if len(r.allowChats) == 0 {
		return true
	}
	_, ok := r.allowChats[chat.Key()]
	if ok {
		return true
	}
	_, ok = r.allowChats[chat.ChatID]
	return ok
}
