package daemon

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (r *Router) handlePlainText(ctx context.Context, message IncomingMessage, text string) error {
	if isWhatsAppChat(message.Chat) && message.IsFromSelf && r.plainText.WhatsAppSelfChatCommandOnly {
		r.logInboundKind(ctx, message, "", "", "input")
		return r.replyBus.Reply(ctx, message.Chat, "", "usage", "WhatsApp self-chat disables plain text to avoid reply loops. Use /send <text>, /enter <text>, /keys <key...>, or reply to a prompt.")
	}
	if r.plainText.Mode == plainTextModeExecute {
		return r.executeText(ctx, message, text, "input")
	}
	return r.sendText(ctx, message, text, "input")
}

func (r *Router) sendText(ctx context.Context, message IncomingMessage, text string, inboundKind string) error {
	chat, paneKey, err := r.currentPaneForInbound(ctx, message, inboundKind)
	if err != nil {
		return r.replyCurrentPaneError(ctx, message, paneKey, err)
	}
	if err := r.service.SendManaged(ctx, paneKey, text, false); err != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("send failed: %v", err))
	}
	return r.replyBus.Reply(ctx, chat, paneKey, "send", fmt.Sprintf("sent to %s", paneKey))
}

func (r *Router) handleSend(ctx context.Context, message IncomingMessage, args string) error {
	text := strings.TrimSpace(args)
	if text == "" {
		_, paneKey, err := r.currentPaneForInbound(ctx, message, "command")
		if err != nil {
			return r.replyCurrentPaneError(ctx, message, paneKey, err)
		}
		return r.promptForCommandInput(ctx, message, "send")
	}
	return r.sendText(ctx, message, text, "command")
}

// executeText keeps the execute path local to the daemon:
//
//	baseline snapshot -> send text + Enter -> bounded poll for visible change
//	                 \-> timeout/no change => explicit fallback message
//
// "Visible change" here is intentionally heuristic. We compare plain tmux
// capture-pane text snapshots, which is good enough for relay-mode operator
// feedback but is not the same thing as shell command completion detection.
func (r *Router) executeText(ctx context.Context, message IncomingMessage, text string, inboundKind string) error {
	chat, paneKey, err := r.currentPaneForInbound(ctx, message, inboundKind)
	if err != nil {
		return r.replyCurrentPaneError(ctx, message, paneKey, err)
	}
	baseline, baselineErr := r.executeBaselineSnapshot(ctx, paneKey)
	if err := r.service.SendManaged(ctx, paneKey, text, true); err != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("send failed: %v", err))
	}
	if !r.shouldEchoExecuteSnapshot() {
		return r.replyEnterSent(ctx, chat, paneKey)
	}
	if baselineErr != nil {
		return r.replyExecuteSnapshotError(ctx, chat, paneKey, baselineErr)
	}
	body, changed, pollErr := r.waitForExecuteSnapshot(ctx, paneKey, baseline)
	if pollErr != nil {
		return r.replyExecuteSnapshotError(ctx, chat, paneKey, pollErr)
	}
	return r.replyExecuteResult(ctx, chat, paneKey, body, changed)
}

func (r *Router) waitForExecuteSnapshot(ctx context.Context, paneKey string, baseline string) (string, bool, error) {
	deadline := time.Now().Add(r.plainText.EchoTimeout)
	var lastBody string
	var lastErr error

	for {
		wait := r.plainText.EchoDelay
		if remaining := time.Until(deadline); remaining <= 0 {
			break
		} else if wait > remaining {
			wait = remaining
		}
		if err := waitForDuration(ctx, wait); err != nil {
			return "", false, err
		}

		body, err := r.service.Snapshot(ctx, paneKey, r.plainText.EchoLines)
		if err != nil {
			lastErr = err
		} else {
			lastBody = body
			// Known limitation: exact string inequality is a relay-mode heuristic
			// for "something visibly changed in the pane", not a guarantee that the
			// command produced meaningful new output.
			if body != baseline {
				return body, true, nil
			}
		}
		if time.Now().After(deadline) {
			break
		}
	}

	if lastErr != nil && strings.TrimSpace(lastBody) == "" {
		return "", false, lastErr
	}
	return lastBody, false, nil
}

func waitForDuration(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (r *Router) handleKeys(ctx context.Context, message IncomingMessage, args string) error {
	chat, paneKey, err := r.currentPaneForInbound(ctx, message, "command")
	if err != nil {
		return r.replyCurrentPaneError(ctx, message, paneKey, err)
	}
	keys, err := parseKeysArgs(args)
	if err != nil {
		if strings.TrimSpace(args) == "" {
			return r.promptForCommandInput(ctx, message, "keys")
		}
		return r.replyBus.Reply(ctx, chat, paneKey, "usage", fmt.Sprintf("%v\n\n%s", err, keysUsage(r.commandPrefix(chat))))
	}
	if err := r.service.SendKeysManaged(ctx, paneKey, keys...); err != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("send keys failed: %v", err))
	}
	return r.replyBus.Reply(ctx, chat, paneKey, "keys", fmt.Sprintf("sent keys %s to %s", strings.Join(keys, " "), paneKey))
}

func (r *Router) handleEnter(ctx context.Context, message IncomingMessage, args string) error {
	text := strings.TrimSpace(args)
	if text != "" {
		return r.executeText(ctx, message, text, "command")
	}

	chat, paneKey, err := r.currentPaneForInbound(ctx, message, "command")
	if err != nil {
		return r.replyCurrentPaneError(ctx, message, paneKey, err)
	}
	if err := r.service.EnterManaged(ctx, paneKey); err != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("enter failed: %v", err))
	}
	return r.replyBus.Reply(ctx, chat, paneKey, "enter", fmt.Sprintf("sent Enter to %s", paneKey))
}

func (r *Router) handleCtrlC(ctx context.Context, message IncomingMessage) error {
	chat, paneKey, err := r.currentPaneForInbound(ctx, message, "command")
	if err != nil {
		return r.replyCurrentPaneError(ctx, message, paneKey, err)
	}
	if err := r.service.CtrlCManaged(ctx, paneKey); err != nil {
		return r.replyBus.Reply(ctx, chat, paneKey, "error", fmt.Sprintf("ctrl-c failed: %v", err))
	}
	return r.replyBus.Reply(ctx, chat, paneKey, "ctrl-c", fmt.Sprintf("sent Ctrl-C to %s", paneKey))
}
