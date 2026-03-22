package daemon

import (
	"context"
	"fmt"
	"strings"
)

func (s *Store) BindPane(ctx context.Context, chat ChatRef, paneKey string) error {
	chat = chat.Normalized()
	if !chat.Valid() {
		return fmt.Errorf("chat ref is required")
	}
	paneKey = strings.TrimSpace(paneKey)
	if paneKey == "" {
		return fmt.Errorf("pane key is required")
	}
	query := fmt.Sprintf(`
INSERT OR IGNORE INTO chat_bindings (platform, chat_id, pane_key)
VALUES (%s, %s, %s);
`, sqlString(chat.Platform), sqlString(chat.ChatID), sqlString(paneKey))
	return s.exec(ctx, query)
}

func (s *Store) UnbindPane(ctx context.Context, chat ChatRef, paneKey string) error {
	chat = chat.Normalized()
	query := fmt.Sprintf(`
DELETE FROM chat_bindings
WHERE platform = %s AND chat_id = %s AND pane_key = %s;
UPDATE chat_state
SET current_pane_key = '', updated_at = CURRENT_TIMESTAMP
WHERE platform = %s AND chat_id = %s AND current_pane_key = %s;
`, sqlString(chat.Platform), sqlString(chat.ChatID), sqlString(strings.TrimSpace(paneKey)), sqlString(chat.Platform), sqlString(chat.ChatID), sqlString(strings.TrimSpace(paneKey)))
	return s.exec(ctx, query)
}

func (s *Store) UnbindPaneEverywhere(ctx context.Context, paneKey string) error {
	query := fmt.Sprintf(`
DELETE FROM chat_bindings
WHERE pane_key = %s;
UPDATE chat_state
SET current_pane_key = '', updated_at = CURRENT_TIMESTAMP
WHERE current_pane_key = %s;
`, sqlString(strings.TrimSpace(paneKey)), sqlString(strings.TrimSpace(paneKey)))
	return s.exec(ctx, query)
}

func (s *Store) ListBindings(ctx context.Context, chat ChatRef) ([]string, error) {
	chat = chat.Normalized()
	type row struct {
		PaneKey string `json:"pane_key"`
	}
	var rows []row
	query := fmt.Sprintf(`
SELECT pane_key
FROM chat_bindings
WHERE platform = %s AND chat_id = %s
ORDER BY pane_key;
`, sqlString(chat.Platform), sqlString(chat.ChatID))
	if err := s.queryJSON(ctx, query, &rows); err != nil {
		return nil, err
	}
	result := make([]string, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.PaneKey)
	}
	return result, nil
}

func (s *Store) SetCurrentPane(ctx context.Context, chat ChatRef, paneKey string) error {
	chat = chat.Normalized()
	query := fmt.Sprintf(`
INSERT INTO chat_state (platform, chat_id, current_pane_key, updated_at)
VALUES (%s, %s, %s, CURRENT_TIMESTAMP)
ON CONFLICT(platform, chat_id) DO UPDATE SET
  current_pane_key = excluded.current_pane_key,
  updated_at = CURRENT_TIMESTAMP;
`, sqlString(chat.Platform), sqlString(chat.ChatID), sqlString(strings.TrimSpace(paneKey)))
	return s.exec(ctx, query)
}

func (s *Store) CurrentPane(ctx context.Context, chat ChatRef) (string, error) {
	chat = chat.Normalized()
	type row struct {
		CurrentPaneKey string `json:"current_pane_key"`
	}
	var rows []row
	query := fmt.Sprintf(`
SELECT current_pane_key
FROM chat_state
WHERE platform = %s AND chat_id = %s
LIMIT 1;
`, sqlString(chat.Platform), sqlString(chat.ChatID))
	if err := s.queryJSON(ctx, query, &rows); err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	return strings.TrimSpace(rows[0].CurrentPaneKey), nil
}
