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

func (r *Router) shouldEchoExecuteSnapshot() bool {
	return r.plainText.Echo == plainTextEchoSnapshot
}

func (r *Router) executeBaselineSnapshot(ctx context.Context, paneKey string) (string, error) {
	if !r.shouldEchoExecuteSnapshot() {
		return "", nil
	}
	return r.service.Snapshot(ctx, paneKey, r.plainText.EchoLines)
}

func (r *Router) executeNoOutputMessage(chat ChatRef, paneKey string) string {
	snapshotUsage := formatCommandUsage(r.commandPrefix(chat), "snapshot text")
	followUsage := formatCommandUsage(r.commandPrefix(chat), "follow on")
	return fmt.Sprintf("sent to %s and pressed Enter; no visible output yet. Try %s or %s.", paneKey, snapshotUsage, followUsage)
}

func (r *Router) replyFollowUsage(ctx context.Context, message IncomingMessage) error {
	r.logInbound(ctx, message, "", "")
	return r.replyBus.Reply(ctx, message.Chat, "", "usage", "usage: "+formatCommandUsage(r.commandPrefix(message.Chat), "follow on [interval]|off"))
}

func (r *Router) replyFollowParseError(ctx context.Context, message IncomingMessage, args string) error {
	if strings.TrimSpace(args) == "" {
		r.logInbound(ctx, message, "", "")
		return r.promptForCommandInput(ctx, message, "follow")
	}
	return r.replyFollowUsage(ctx, message)
}

func (r *Router) replyFollowEnabled(ctx context.Context, chat ChatRef, paneKey string, opts FollowOptions) error {
	return r.replyBus.Reply(ctx, chat, paneKey, "follow", fmt.Sprintf("follow enabled for %s (min interval %s)", paneKey, opts.MinInterval))
}

func (r *Router) replyFollowDisabled(ctx context.Context, chat ChatRef, paneKey string, disabled bool) error {
	if !disabled {
		return r.replyBus.Reply(ctx, chat, paneKey, "follow", "follow is already off")
	}
	return r.replyBus.Reply(ctx, chat, paneKey, "follow", "follow disabled")
}
