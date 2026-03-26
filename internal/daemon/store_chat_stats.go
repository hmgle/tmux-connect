package daemon

import (
	"context"
	"fmt"
	"strings"
)

func (s *Store) ListChats(ctx context.Context, platform string) ([]string, error) {
	platform = strings.TrimSpace(strings.ToLower(platform))
	rows, err := s.db.QueryContext(ctx, `
SELECT chat_id
FROM (
  SELECT chat_id FROM chat_state WHERE platform = ?
  UNION
  SELECT chat_id FROM chat_bindings WHERE platform = ?
)
ORDER BY chat_id;
`, platform, platform)
	if err != nil {
		return nil, fmt.Errorf("sqlite query failed: %w", err)
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var chatID string
		if err := rows.Scan(&chatID); err != nil {
			return nil, fmt.Errorf("sqlite scan: %w", err)
		}
		result = append(result, strings.TrimSpace(chatID))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite rows: %w", err)
	}
	return result, nil
}

func (s *Store) Stats(ctx context.Context) (StoreStats, error) {
	var stats StoreStats
	err := s.db.QueryRowContext(ctx, `
SELECT
  (SELECT COUNT(*) FROM (
     SELECT platform, chat_id FROM chat_state
     UNION
     SELECT platform, chat_id FROM chat_bindings
   )) AS chats,
  (SELECT COUNT(*) FROM chat_bindings) AS bindings,
  (SELECT COUNT(*) FROM message_log) AS messages;
`).Scan(&stats.Chats, &stats.Bindings, &stats.Messages)
	if err != nil {
		return StoreStats{}, fmt.Errorf("sqlite query failed: %w", err)
	}
	return stats, nil
}
