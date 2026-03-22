package daemon

import (
	"context"
	"fmt"
	"strings"
)

func (r *Router) replySnapshotForMode(ctx context.Context, chat ChatRef, paneKey string, lines int, mode snapshotMode) error {
	body, err := r.service.Snapshot(ctx, paneKey, lines)
	if err != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("snapshot failed: %v", err))
	}
	if mode == snapshotModeText {
		return r.replySnapshotText(ctx, chat, paneKey, body)
	}
	richBody, richErr := r.service.SnapshotRich(ctx, paneKey, lines)
	if richErr == nil && strings.TrimSpace(richBody) != "" {
		return r.replyBus.ReplySnapshot(ctx, chat, paneKey, body, richBody)
	}
	return r.replySnapshotText(ctx, chat, paneKey, body)
}

func (r *Router) replySnapshotText(ctx context.Context, chat ChatRef, paneKey string, body string) error {
	return r.replyBus.Reply(ctx, chat, paneKey, "snapshot", formatFollowMessage(paneKey, body, 3500))
}

func (r *Router) replyEnterSent(ctx context.Context, chat ChatRef, paneKey string) error {
	return r.replyBus.Reply(ctx, chat, paneKey, "enter", fmt.Sprintf("sent to %s and pressed Enter", paneKey))
}

func (r *Router) replyExecuteSnapshotError(ctx context.Context, chat ChatRef, paneKey string, err error) error {
	return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("sent to %s and pressed Enter, but snapshot failed: %v", paneKey, err))
}

func (r *Router) replyExecuteResult(ctx context.Context, chat ChatRef, paneKey string, body string, changed bool) error {
	if !changed {
		return r.replyBus.Reply(ctx, chat, paneKey, "enter", r.executeNoOutputMessage(chat, paneKey))
	}
	return r.replySnapshotText(ctx, chat, paneKey, body)
}

func (r *Router) executeNoOutputMessage(chat ChatRef, paneKey string) string {
	snapshotUsage := formatCommandUsage(r.commandPrefix(chat), "snapshot text")
	followUsage := formatCommandUsage(r.commandPrefix(chat), "follow on")
	return fmt.Sprintf("sent to %s and pressed Enter; no visible output yet. Try %s or %s.", paneKey, snapshotUsage, followUsage)
}
