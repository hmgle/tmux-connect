package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (s *Store) ListChats(ctx context.Context, platform string) ([]string, error) {
	platform = strings.TrimSpace(strings.ToLower(platform))
	type row struct {
		ChatID string `json:"chat_id"`
	}
	var rows []row
	query := fmt.Sprintf(`
SELECT chat_id
FROM (
  SELECT chat_id FROM chat_state WHERE platform = %s
  UNION
  SELECT chat_id FROM chat_bindings WHERE platform = %s
)
ORDER BY chat_id;
`, sqlString(platform), sqlString(platform))
	if err := s.queryJSON(ctx, query, &rows); err != nil {
		return nil, err
	}
	result := make([]string, 0, len(rows))
	for _, row := range rows {
		result = append(result, strings.TrimSpace(row.ChatID))
	}
	return result, nil
}

func (s *Store) Stats(ctx context.Context) (StoreStats, error) {
	type row struct {
		Chats    json.Number `json:"chats"`
		Bindings json.Number `json:"bindings"`
		Messages json.Number `json:"messages"`
	}
	var rows []row
	query := `
SELECT
  (SELECT COUNT(*) FROM (
     SELECT platform, chat_id FROM chat_state
     UNION
     SELECT platform, chat_id FROM chat_bindings
   )) AS chats,
  (SELECT COUNT(*) FROM chat_bindings) AS bindings,
  (SELECT COUNT(*) FROM message_log) AS messages;
`
	if err := s.queryJSON(ctx, query, &rows); err != nil {
		return StoreStats{}, err
	}
	if len(rows) == 0 {
		return StoreStats{}, nil
	}
	chats, err := numberToInt(rows[0].Chats)
	if err != nil {
		return StoreStats{}, err
	}
	bindings, err := numberToInt(rows[0].Bindings)
	if err != nil {
		return StoreStats{}, err
	}
	messages, err := numberToInt(rows[0].Messages)
	if err != nil {
		return StoreStats{}, err
	}
	return StoreStats{Chats: chats, Bindings: bindings, Messages: messages}, nil
}
