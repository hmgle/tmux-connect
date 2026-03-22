package daemon

import (
	"context"
	"fmt"
	"strings"
)

const sessionSelectColumns = "session_key, platform, chat_id, pane_key, agent, agent_session_id, agent_thread_id, last_inbound_message_id, last_outbound_message_id"

func (s *Store) SessionByKey(ctx context.Context, sessionKey string) (SessionRecord, error) {
	return s.querySingleSession(ctx, fmt.Sprintf(`
SELECT %s
FROM sessions
WHERE session_key = %s
LIMIT 1;
`, sessionSelectColumns, sqlString(strings.TrimSpace(sessionKey))))
}

func (s *Store) LatestSessionByChatPane(ctx context.Context, chat ChatRef, paneKey string) (SessionRecord, error) {
	chat = chat.Normalized()
	return s.querySingleSession(ctx, fmt.Sprintf(`
SELECT %s
FROM sessions
WHERE platform = %s AND chat_id = %s AND pane_key = %s
ORDER BY updated_at DESC
LIMIT 1;
`, sessionSelectColumns, sqlString(chat.Platform), sqlString(chat.ChatID), sqlString(strings.TrimSpace(paneKey))))
}

func (s *Store) querySingleSession(ctx context.Context, query string) (SessionRecord, error) {
	var rows []sessionRow
	if err := s.queryJSON(ctx, query, &rows); err != nil {
		return SessionRecord{}, err
	}
	if len(rows) == 0 {
		return SessionRecord{}, nil
	}
	return rows[0].toRecord()
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
