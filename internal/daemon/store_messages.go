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

func (s *Store) CreateMessageLink(ctx context.Context, record MessageLinkRecord) error {
	return s.exec(ctx, createMessageLinkStatement(record))
}
