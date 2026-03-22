package daemon

import (
	"fmt"
	"strings"
)

func logMessageStatement(record MessageRecord) string {
	chat := record.Chat.Normalized()
	return fmt.Sprintf(`
INSERT INTO message_log (
  platform,
  chat_id,
  pane_key,
  direction,
  kind,
  platform_message_id,
  thread_id,
  body_preview
)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s);
`, sqlString(chat.Platform), sqlString(chat.ChatID), sqlString(strings.TrimSpace(record.PaneKey)), sqlString(strings.TrimSpace(record.Direction)), sqlString(strings.TrimSpace(record.Kind)), sqlString(strings.TrimSpace(record.PlatformMessageID)), sqlString(strings.TrimSpace(record.ThreadID)), sqlString(truncatePreview(record.BodyPreview)))
}

func touchSessionInboundStatement(sessionKey string, messageID string) string {
	return fmt.Sprintf(`
UPDATE sessions
SET last_inbound_message_id = %s,
    updated_at = CURRENT_TIMESTAMP
WHERE session_key = %s;
`, sqlString(strings.TrimSpace(messageID)), sqlString(strings.TrimSpace(sessionKey)))
}

func touchSessionOutboundStatement(sessionKey string, messageID string) string {
	return fmt.Sprintf(`
UPDATE sessions
SET last_outbound_message_id = %s,
    updated_at = CURRENT_TIMESTAMP
WHERE session_key = %s;
`, sqlString(strings.TrimSpace(messageID)), sqlString(strings.TrimSpace(sessionKey)))
}

func updateSessionThreadStatement(sessionKey string, threadID string) string {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return ""
	}
	return fmt.Sprintf(`
UPDATE sessions
SET agent_thread_id = %s,
    updated_at = CURRENT_TIMESTAMP
WHERE session_key = %s;
`, sqlString(threadID), sqlString(strings.TrimSpace(sessionKey)))
}

func createMessageLinkStatement(record MessageLinkRecord) string {
	return fmt.Sprintf(`
INSERT INTO message_links (
  platform,
  chat_id,
  pane_key,
  session_key,
  kind,
  inbound_message_id,
  outbound_message_id,
  reply_to_message_id
)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s);
`, sqlString(strings.TrimSpace(record.Platform)), sqlString(strings.TrimSpace(record.ChatID)), sqlString(strings.TrimSpace(record.PaneKey)), sqlString(strings.TrimSpace(record.SessionKey)), sqlString(strings.TrimSpace(record.Kind)), sqlString(strings.TrimSpace(record.InboundMessageID)), sqlString(strings.TrimSpace(record.OutboundMessageID)), sqlString(strings.TrimSpace(record.ReplyToMessageID)))
}

func ensureSessionStatement(chat ChatRef, paneKey string, agent string, sessionKey string) string {
	chat = chat.Normalized()
	return fmt.Sprintf(`
INSERT INTO sessions (
  session_key,
  platform,
  chat_id,
  pane_key,
  agent,
  agent_session_id,
  agent_thread_id,
  last_inbound_message_id,
  last_outbound_message_id
)
VALUES (%s, %s, %s, %s, %s, '', '', '', '')
ON CONFLICT(session_key) DO UPDATE SET
  pane_key = excluded.pane_key,
  agent = CASE
    WHEN trim(excluded.agent) <> '' THEN excluded.agent
    ELSE sessions.agent
  END,
  updated_at = CURRENT_TIMESTAMP;
`, sqlString(strings.TrimSpace(sessionKey)), sqlString(chat.Platform), sqlString(chat.ChatID), sqlString(strings.TrimSpace(paneKey)), sqlString(strings.TrimSpace(agent)))
}

func isReturningUnsupported(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "RETURNING") && strings.Contains(strings.ToLower(message), "syntax error")
}
