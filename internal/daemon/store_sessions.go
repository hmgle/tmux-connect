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
	record, err := scanSession(s.db.QueryRowContext(ctx, `
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
VALUES (?, ?, ?, ?, ?, '', '', '', '')
ON CONFLICT(session_key) DO UPDATE SET
  pane_key = excluded.pane_key,
  agent = CASE
    WHEN trim(excluded.agent) <> '' THEN excluded.agent
    ELSE sessions.agent
  END,
  updated_at = CURRENT_TIMESTAMP
RETURNING session_key, platform, chat_id, pane_key, agent, agent_session_id, agent_thread_id, last_inbound_message_id, last_outbound_message_id;
`, sessionKey, chat.Platform, chat.ChatID, paneKey, agent))
	if err == nil {
		return record, nil
	}
	if !isReturningUnsupported(err) {
		return SessionRecord{}, fmt.Errorf("sqlite query failed: %w", err)
	}

	if err := s.execArgs(ctx, `
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
VALUES (?, ?, ?, ?, ?, '', '', '', '')
ON CONFLICT(session_key) DO UPDATE SET
  pane_key = excluded.pane_key,
  agent = CASE
    WHEN trim(excluded.agent) <> '' THEN excluded.agent
    ELSE sessions.agent
  END,
  updated_at = CURRENT_TIMESTAMP;
`, sessionKey, chat.Platform, chat.ChatID, paneKey, agent); err != nil {
		return SessionRecord{}, err
	}
	return s.SessionByKey(ctx, sessionKey)
}

func (s *Store) TouchSessionInbound(ctx context.Context, sessionKey string, messageID string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE sessions
SET last_inbound_message_id = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE session_key = ?;
`, strings.TrimSpace(messageID), strings.TrimSpace(sessionKey))
	if err != nil {
		return fmt.Errorf("sqlite exec failed: %w", err)
	}
	return nil
}

func (s *Store) TouchSessionOutbound(ctx context.Context, sessionKey string, messageID string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE sessions
SET last_outbound_message_id = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE session_key = ?;
`, strings.TrimSpace(messageID), strings.TrimSpace(sessionKey))
	if err != nil {
		return fmt.Errorf("sqlite exec failed: %w", err)
	}
	return nil
}

func isReturningUnsupported(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "RETURNING") && strings.Contains(strings.ToLower(message), "syntax error")
}
