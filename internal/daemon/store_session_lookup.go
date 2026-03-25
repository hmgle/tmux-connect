package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const sessionSelectColumns = "session_key, platform, chat_id, pane_key, agent, agent_session_id, agent_thread_id, last_inbound_message_id, last_outbound_message_id"

func (s *Store) SessionByKey(ctx context.Context, sessionKey string) (SessionRecord, error) {
	return s.querySingleSession(ctx, `
SELECT %s
FROM sessions
WHERE session_key = ?
LIMIT 1;
`, strings.TrimSpace(sessionKey))
}

func (s *Store) LatestSessionByChatPane(ctx context.Context, chat ChatRef, paneKey string) (SessionRecord, error) {
	chat = chat.Normalized()
	return s.querySingleSession(ctx, `
SELECT %s
FROM sessions
WHERE platform = ? AND chat_id = ? AND pane_key = ?
ORDER BY updated_at DESC
LIMIT 1;
`, chat.Platform, chat.ChatID, strings.TrimSpace(paneKey))
}

func (s *Store) querySingleSession(ctx context.Context, query string, args ...any) (SessionRecord, error) {
	query = fmt.Sprintf(query, sessionSelectColumns)
	record, err := scanSession(s.db.QueryRowContext(ctx, query, args...))
	if err == sql.ErrNoRows {
		return SessionRecord{}, nil
	}
	if err != nil {
		return SessionRecord{}, fmt.Errorf("sqlite query failed: %w", err)
	}
	return record, nil
}

type sessionScanner interface {
	Scan(dest ...any) error
}

func scanSession(scanner sessionScanner) (SessionRecord, error) {
	var record SessionRecord
	err := scanner.Scan(
		&record.SessionKey,
		&record.Platform,
		&record.ChatID,
		&record.PaneKey,
		&record.Agent,
		&record.AgentSessionID,
		&record.AgentThreadID,
		&record.LastInboundMessageID,
		&record.LastOutboundMessageID,
	)
	if err != nil {
		return SessionRecord{}, err
	}
	record.ChatID = strings.TrimSpace(record.ChatID)
	record.LastInboundMessageID = strings.TrimSpace(record.LastInboundMessageID)
	record.LastOutboundMessageID = strings.TrimSpace(record.LastOutboundMessageID)
	return record, nil
}

func sessionKeyFor(chat ChatRef, paneKey string) string {
	chat = chat.Normalized()
	return chat.Platform + ":" + chat.ChatID + ":" + strings.TrimSpace(paneKey)
}
