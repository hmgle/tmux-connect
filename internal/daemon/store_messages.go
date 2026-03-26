package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func (s *Store) LogMessage(ctx context.Context, record MessageRecord) error {
	return s.execLogMessage(ctx, nil, record)
}

func (s *Store) RecordOutbound(ctx context.Context, record MessageRecord, link *MessageLinkRecord) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if err := s.execLogMessage(ctx, tx, record); err != nil {
			return err
		}
		if link != nil && strings.TrimSpace(link.SessionKey) != "" {
			if err := s.touchSessionOutboundTx(ctx, tx, strings.TrimSpace(link.SessionKey), link.OutboundMessageID); err != nil {
				return err
			}
			if err := s.createMessageLinkTx(ctx, tx, *link); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) RecordInbound(ctx context.Context, record MessageRecord, platform string, agent string) error {
	paneKey := strings.TrimSpace(record.PaneKey)
	platform = strings.TrimSpace(platform)
	if paneKey != "" && platform == "" {
		return fmt.Errorf("platform is required")
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if err := s.execLogMessage(ctx, tx, record); err != nil {
			return err
		}
		if paneKey == "" {
			return nil
		}
		sessionKey := sessionKeyFor(record.Chat, paneKey)
		if err := s.ensureSessionTx(ctx, tx, record.Chat, paneKey, strings.TrimSpace(agent), sessionKey); err != nil {
			return err
		}
		if err := s.updateSessionThreadTx(ctx, tx, sessionKey, record.ThreadID); err != nil {
			return err
		}
		if err := s.touchSessionInboundTx(ctx, tx, sessionKey, record.PlatformMessageID); err != nil {
			return err
		}
		return s.createMessageLinkTx(ctx, tx, MessageLinkRecord{
			Platform:         platform,
			ChatID:           record.Chat.ChatID,
			PaneKey:          paneKey,
			SessionKey:       sessionKey,
			Kind:             strings.TrimSpace(record.Kind),
			InboundMessageID: record.PlatformMessageID,
		})
	})
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

	var present bool
	err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM message_log
  WHERE platform = ? AND chat_id = ? AND thread_id = ?
) AS present;
`, chat.Platform, chat.ChatID, threadID).Scan(&present)
	if err != nil {
		return false, fmt.Errorf("sqlite query failed: %w", err)
	}
	return present, nil
}

func (s *Store) CreateMessageLink(ctx context.Context, record MessageLinkRecord) error {
	return s.createMessageLinkTx(ctx, nil, record)
}

func (s *Store) execLogMessage(ctx context.Context, tx *sql.Tx, record MessageRecord) error {
	chat := record.Chat.Normalized()
	_, err := s.execer(tx).ExecContext(ctx, `
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
VALUES (?, ?, ?, ?, ?, ?, ?, ?);
`, chat.Platform, chat.ChatID, strings.TrimSpace(record.PaneKey), strings.TrimSpace(record.Direction), strings.TrimSpace(record.Kind), strings.TrimSpace(record.PlatformMessageID), strings.TrimSpace(record.ThreadID), truncatePreview(record.BodyPreview))
	if err != nil {
		return fmt.Errorf("sqlite exec failed: %w", err)
	}
	return nil
}

func (s *Store) ensureSessionTx(ctx context.Context, tx *sql.Tx, chat ChatRef, paneKey string, agent string, sessionKey string) error {
	chat = chat.Normalized()
	_, err := s.execer(tx).ExecContext(ctx, `
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
`, strings.TrimSpace(sessionKey), chat.Platform, chat.ChatID, strings.TrimSpace(paneKey), strings.TrimSpace(agent))
	if err != nil {
		return fmt.Errorf("sqlite exec failed: %w", err)
	}
	return nil
}

func (s *Store) touchSessionInboundTx(ctx context.Context, tx *sql.Tx, sessionKey string, messageID string) error {
	_, err := s.execer(tx).ExecContext(ctx, `
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

func (s *Store) touchSessionOutboundTx(ctx context.Context, tx *sql.Tx, sessionKey string, messageID string) error {
	_, err := s.execer(tx).ExecContext(ctx, `
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

func (s *Store) updateSessionThreadTx(ctx context.Context, tx *sql.Tx, sessionKey string, threadID string) error {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil
	}
	_, err := s.execer(tx).ExecContext(ctx, `
UPDATE sessions
SET agent_thread_id = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE session_key = ?;
`, threadID, strings.TrimSpace(sessionKey))
	if err != nil {
		return fmt.Errorf("sqlite exec failed: %w", err)
	}
	return nil
}

func (s *Store) createMessageLinkTx(ctx context.Context, tx *sql.Tx, record MessageLinkRecord) error {
	_, err := s.execer(tx).ExecContext(ctx, `
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
VALUES (?, ?, ?, ?, ?, ?, ?, ?);
`, strings.TrimSpace(record.Platform), strings.TrimSpace(record.ChatID), strings.TrimSpace(record.PaneKey), strings.TrimSpace(record.SessionKey), strings.TrimSpace(record.Kind), strings.TrimSpace(record.InboundMessageID), strings.TrimSpace(record.OutboundMessageID), strings.TrimSpace(record.ReplyToMessageID))
	if err != nil {
		return fmt.Errorf("sqlite exec failed: %w", err)
	}
	return nil
}

type sqlExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s *Store) execer(tx *sql.Tx) sqlExecer {
	if tx != nil {
		return tx
	}
	return s.db
}
