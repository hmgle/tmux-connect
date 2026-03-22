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

func (s *Store) TouchSessionInbound(ctx context.Context, sessionKey string, messageID string) error {
	return s.exec(ctx, touchSessionInboundStatement(sessionKey, messageID))
}

func (s *Store) TouchSessionOutbound(ctx context.Context, sessionKey string, messageID string) error {
	return s.exec(ctx, touchSessionOutboundStatement(sessionKey, messageID))
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
