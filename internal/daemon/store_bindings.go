package daemon

import (
	"context"
	"database/sql"
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
	return s.execArgs(ctx, `
INSERT OR IGNORE INTO chat_bindings (platform, chat_id, pane_key)
VALUES (?, ?, ?);
`, chat.Platform, chat.ChatID, paneKey)
}

func (s *Store) UnbindPane(ctx context.Context, chat ChatRef, paneKey string) error {
	chat = chat.Normalized()
	paneKey = strings.TrimSpace(paneKey)
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
DELETE FROM chat_bindings
WHERE platform = ? AND chat_id = ? AND pane_key = ?;
`, chat.Platform, chat.ChatID, paneKey); err != nil {
			return fmt.Errorf("sqlite exec failed: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE chat_state
SET current_pane_key = '', updated_at = CURRENT_TIMESTAMP
WHERE platform = ? AND chat_id = ? AND current_pane_key = ?;
`, chat.Platform, chat.ChatID, paneKey); err != nil {
			return fmt.Errorf("sqlite exec failed: %w", err)
		}
		return nil
	})
}

func (s *Store) UnbindPaneEverywhere(ctx context.Context, paneKey string) error {
	paneKey = strings.TrimSpace(paneKey)
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
DELETE FROM chat_bindings
WHERE pane_key = ?;
`, paneKey); err != nil {
			return fmt.Errorf("sqlite exec failed: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE chat_state
SET current_pane_key = '', updated_at = CURRENT_TIMESTAMP
WHERE current_pane_key = ?;
`, paneKey); err != nil {
			return fmt.Errorf("sqlite exec failed: %w", err)
		}
		return nil
	})
}

func (s *Store) ListBindings(ctx context.Context, chat ChatRef) ([]string, error) {
	chat = chat.Normalized()
	rows, err := s.db.QueryContext(ctx, `
SELECT pane_key
FROM chat_bindings
WHERE platform = ? AND chat_id = ?
ORDER BY pane_key;
`, chat.Platform, chat.ChatID)
	if err != nil {
		return nil, fmt.Errorf("sqlite query failed: %w", err)
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var paneKey string
		if err := rows.Scan(&paneKey); err != nil {
			return nil, fmt.Errorf("sqlite scan: %w", err)
		}
		result = append(result, paneKey)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite rows: %w", err)
	}
	return result, nil
}

func (s *Store) SetCurrentPane(ctx context.Context, chat ChatRef, paneKey string) error {
	chat = chat.Normalized()
	return s.execArgs(ctx, `
INSERT INTO chat_state (platform, chat_id, current_pane_key, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(platform, chat_id) DO UPDATE SET
  current_pane_key = excluded.current_pane_key,
  updated_at = CURRENT_TIMESTAMP;
`, chat.Platform, chat.ChatID, strings.TrimSpace(paneKey))
}

func (s *Store) CurrentPane(ctx context.Context, chat ChatRef) (string, error) {
	chat = chat.Normalized()
	var currentPaneKey string
	err := s.db.QueryRowContext(ctx, `
SELECT current_pane_key
FROM chat_state
WHERE platform = ? AND chat_id = ?
LIMIT 1;
`, chat.Platform, chat.ChatID).Scan(&currentPaneKey)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("sqlite query failed: %w", err)
	}
	return strings.TrimSpace(currentPaneKey), nil
}
