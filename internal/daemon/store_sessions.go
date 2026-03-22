package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (s *Store) LogMessage(ctx context.Context, record MessageRecord) error {
	return s.exec(ctx, logMessageStatement(record))
}

func (s *Store) RecordOutbound(ctx context.Context, record MessageRecord, link *MessageLinkRecord) error {
	statements := []string{logMessageStatement(record)}
	if link != nil && strings.TrimSpace(link.SessionKey) != "" {
		statements = append(statements,
			touchSessionOutboundStatement(strings.TrimSpace(link.SessionKey), link.OutboundMessageID),
			createMessageLinkStatement(*link),
		)
	}
	return s.exec(ctx, wrapTransaction(statements...))
}

func (s *Store) RecordInbound(ctx context.Context, record MessageRecord, platform string, agent string) error {
	statements := []string{logMessageStatement(record)}
	paneKey := strings.TrimSpace(record.PaneKey)
	if paneKey != "" {
		platform = strings.TrimSpace(platform)
		if platform == "" {
			return fmt.Errorf("platform is required")
		}
		sessionKey := sessionKeyFor(record.Chat, paneKey)
		statements = append(statements,
			ensureSessionStatement(record.Chat, paneKey, strings.TrimSpace(agent), sessionKey),
			updateSessionThreadStatement(sessionKey, record.ThreadID),
			touchSessionInboundStatement(sessionKey, record.PlatformMessageID),
			createMessageLinkStatement(MessageLinkRecord{
				Platform:         platform,
				ChatID:           record.Chat.ChatID,
				PaneKey:          paneKey,
				SessionKey:       sessionKey,
				Kind:             strings.TrimSpace(record.Kind),
				InboundMessageID: record.PlatformMessageID,
			}),
		)
	}
	return s.exec(ctx, wrapTransaction(statements...))
}

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

func (s *Store) EnsureSession(ctx context.Context, chat ChatRef, paneKey string, agent string) (SessionRecord, error) {
	chat = chat.Normalized()
	paneKey = strings.TrimSpace(paneKey)
	agent = strings.TrimSpace(agent)
	if !chat.Valid() {
		return SessionRecord{}, fmt.Errorf("chat ref is required")
	}
	if paneKey == "" {
		return SessionRecord{}, fmt.Errorf("pane key is required")
	}
	sessionKey := sessionKeyFor(chat, paneKey)
	query := fmt.Sprintf(`
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
  updated_at = CURRENT_TIMESTAMP
RETURNING session_key, platform, chat_id, pane_key, agent, agent_session_id, agent_thread_id, last_inbound_message_id, last_outbound_message_id;
`, sqlString(sessionKey), sqlString(chat.Platform), sqlString(chat.ChatID), sqlString(paneKey), sqlString(agent))
	var rows []sessionRow
	if err := s.queryJSON(ctx, query, &rows); err == nil {
		if len(rows) == 0 {
			return SessionRecord{}, fmt.Errorf("ensure session returned no rows")
		}
		return rows[0].toRecord()
	} else if !isReturningUnsupported(err) {
		return SessionRecord{}, err
	}

	fallbackQuery := fmt.Sprintf(`
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
`, sqlString(sessionKey), sqlString(chat.Platform), sqlString(chat.ChatID), sqlString(paneKey), sqlString(agent))
	if err := s.exec(ctx, fallbackQuery); err != nil {
		return SessionRecord{}, err
	}
	return s.SessionByKey(ctx, sessionKey)
}

func (s *Store) SessionByKey(ctx context.Context, sessionKey string) (SessionRecord, error) {
	var rows []sessionRow
	query := fmt.Sprintf(`
SELECT session_key, platform, chat_id, pane_key, agent, agent_session_id, agent_thread_id, last_inbound_message_id, last_outbound_message_id
FROM sessions
WHERE session_key = %s
LIMIT 1;
`, sqlString(strings.TrimSpace(sessionKey)))
	if err := s.queryJSON(ctx, query, &rows); err != nil {
		return SessionRecord{}, err
	}
	if len(rows) == 0 {
		return SessionRecord{}, nil
	}
	return rows[0].toRecord()
}

func (s *Store) LatestSessionByChatPane(ctx context.Context, chat ChatRef, paneKey string) (SessionRecord, error) {
	chat = chat.Normalized()
	var rows []sessionRow
	query := fmt.Sprintf(`
SELECT session_key, platform, chat_id, pane_key, agent, agent_session_id, agent_thread_id, last_inbound_message_id, last_outbound_message_id
FROM sessions
WHERE platform = %s AND chat_id = %s AND pane_key = %s
ORDER BY updated_at DESC
LIMIT 1;
`, sqlString(chat.Platform), sqlString(chat.ChatID), sqlString(strings.TrimSpace(paneKey)))
	if err := s.queryJSON(ctx, query, &rows); err != nil {
		return SessionRecord{}, err
	}
	if len(rows) == 0 {
		return SessionRecord{}, nil
	}
	return rows[0].toRecord()
}

func (s *Store) HasThread(ctx context.Context, chat ChatRef, threadID string) (bool, error) {
	chat = chat.Normalized()
	threadID = strings.TrimSpace(threadID)
	if !chat.Valid() {
		return false, fmt.Errorf("chat ref is required")
	}
	if threadID == "" {
		return false, nil
	}

	type row struct {
		Present json.Number `json:"present"`
	}
	var rows []row
	query := fmt.Sprintf(`
SELECT EXISTS(
  SELECT 1
  FROM message_log
  WHERE platform = %s AND chat_id = %s AND thread_id = %s
) AS present;
`, sqlString(chat.Platform), sqlString(chat.ChatID), sqlString(threadID))
	if err := s.queryJSON(ctx, query, &rows); err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, nil
	}
	present, err := numberToInt(rows[0].Present)
	if err != nil {
		return false, err
	}
	return present != 0, nil
}

func (s *Store) TouchSessionInbound(ctx context.Context, sessionKey string, messageID string) error {
	return s.exec(ctx, touchSessionInboundStatement(sessionKey, messageID))
}

func touchSessionInboundStatement(sessionKey string, messageID string) string {
	return fmt.Sprintf(`
UPDATE sessions
SET last_inbound_message_id = %s,
    updated_at = CURRENT_TIMESTAMP
WHERE session_key = %s;
`, sqlString(strings.TrimSpace(messageID)), sqlString(strings.TrimSpace(sessionKey)))
}

func (s *Store) TouchSessionOutbound(ctx context.Context, sessionKey string, messageID string) error {
	return s.exec(ctx, touchSessionOutboundStatement(sessionKey, messageID))
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

func (s *Store) CreateMessageLink(ctx context.Context, record MessageLinkRecord) error {
	return s.exec(ctx, createMessageLinkStatement(record))
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

type sessionRow struct {
	SessionKey            string `json:"session_key"`
	Platform              string `json:"platform"`
	ChatID                string `json:"chat_id"`
	PaneKey               string `json:"pane_key"`
	Agent                 string `json:"agent"`
	AgentSessionID        string `json:"agent_session_id"`
	AgentThreadID         string `json:"agent_thread_id"`
	LastInboundMessageID  string `json:"last_inbound_message_id"`
	LastOutboundMessageID string `json:"last_outbound_message_id"`
}

func (r sessionRow) toRecord() (SessionRecord, error) {
	return SessionRecord{
		SessionKey:            r.SessionKey,
		Platform:              r.Platform,
		ChatID:                strings.TrimSpace(r.ChatID),
		PaneKey:               r.PaneKey,
		Agent:                 r.Agent,
		AgentSessionID:        r.AgentSessionID,
		AgentThreadID:         r.AgentThreadID,
		LastInboundMessageID:  strings.TrimSpace(r.LastInboundMessageID),
		LastOutboundMessageID: strings.TrimSpace(r.LastOutboundMessageID),
	}, nil
}

func sessionKeyFor(chat ChatRef, paneKey string) string {
	chat = chat.Normalized()
	return chat.Platform + ":" + chat.ChatID + ":" + strings.TrimSpace(paneKey)
}

func isReturningUnsupported(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "RETURNING") && strings.Contains(strings.ToLower(message), "syntax error")
}
