package daemon

import (
	"context"
	"strings"
)

type interactionReplyContextKey struct{}

type interactionReplyContext struct {
	ID string
}

func withInteractionReplyContext(ctx context.Context, interactionID string) context.Context {
	interactionID = strings.TrimSpace(interactionID)
	if interactionID == "" {
		return ctx
	}
	return context.WithValue(ctx, interactionReplyContextKey{}, interactionReplyContext{ID: interactionID})
}

func applyInteractionReplyContext(ctx context.Context, opts SendOptions) SendOptions {
	if strings.TrimSpace(opts.InteractionID) != "" {
		return opts
	}
	value, ok := ctx.Value(interactionReplyContextKey{}).(interactionReplyContext)
	if !ok {
		return opts
	}
	opts.InteractionID = strings.TrimSpace(value.ID)
	return opts
}
