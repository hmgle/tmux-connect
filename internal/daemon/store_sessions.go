package daemon

import (
	"context"
	"fmt"
	"strings"
)

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

func (s *Store) TouchSessionInbound(ctx context.Context, sessionKey string, messageID string) error {
	return s.exec(ctx, touchSessionInboundStatement(sessionKey, messageID))
}

func (s *Store) TouchSessionOutbound(ctx context.Context, sessionKey string, messageID string) error {
	return s.exec(ctx, touchSessionOutboundStatement(sessionKey, messageID))
}
