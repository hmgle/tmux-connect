package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type Store struct {
	path      string
	sqliteBin string
	mu        sync.Mutex
}

type MessageRecord struct {
	ChatID            int64
	PaneKey           string
	Direction         string
	Kind              string
	TelegramMessageID int64
	BodyPreview       string
}

type StoreStats struct {
	Chats    int
	Bindings int
	Messages int
}

func OpenStore(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("db path is required")
	}
	sqliteBin, err := exec.LookPath("sqlite3")
	if err != nil {
		return nil, fmt.Errorf("sqlite3 not found in PATH: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	store := &Store{
		path:      path,
		sqliteBin: sqliteBin,
	}
	if err := store.init(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) init(ctx context.Context) error {
	schema := `
PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS chat_bindings (
  chat_id INTEGER NOT NULL,
  pane_key TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (chat_id, pane_key)
);
CREATE TABLE IF NOT EXISTS chat_state (
  chat_id INTEGER PRIMARY KEY,
  current_pane_key TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS message_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  chat_id INTEGER NOT NULL,
  pane_key TEXT NOT NULL DEFAULT '',
  direction TEXT NOT NULL,
  kind TEXT NOT NULL,
  telegram_message_id INTEGER NOT NULL DEFAULT 0,
  body_preview TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_message_log_chat_created_at
  ON message_log (chat_id, created_at DESC);
`
	return s.exec(ctx, schema)
}

func (s *Store) BindPane(ctx context.Context, chatID int64, paneKey string) error {
	paneKey = strings.TrimSpace(paneKey)
	if paneKey == "" {
		return fmt.Errorf("pane key is required")
	}
	query := fmt.Sprintf(`
INSERT OR IGNORE INTO chat_bindings (chat_id, pane_key)
VALUES (%d, %s);
`, chatID, sqlString(paneKey))
	return s.exec(ctx, query)
}

func (s *Store) UnbindPane(ctx context.Context, chatID int64, paneKey string) error {
	query := fmt.Sprintf(`
DELETE FROM chat_bindings
WHERE chat_id = %d AND pane_key = %s;
UPDATE chat_state
SET current_pane_key = '', updated_at = CURRENT_TIMESTAMP
WHERE chat_id = %d AND current_pane_key = %s;
`, chatID, sqlString(strings.TrimSpace(paneKey)), chatID, sqlString(strings.TrimSpace(paneKey)))
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

func (s *Store) ListBindings(ctx context.Context, chatID int64) ([]string, error) {
	type row struct {
		PaneKey string `json:"pane_key"`
	}
	var rows []row
	query := fmt.Sprintf(`
SELECT pane_key
FROM chat_bindings
WHERE chat_id = %d
ORDER BY pane_key;
`, chatID)
	if err := s.queryJSON(ctx, query, &rows); err != nil {
		return nil, err
	}
	result := make([]string, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.PaneKey)
	}
	return result, nil
}

func (s *Store) SetCurrentPane(ctx context.Context, chatID int64, paneKey string) error {
	query := fmt.Sprintf(`
INSERT INTO chat_state (chat_id, current_pane_key, updated_at)
VALUES (%d, %s, CURRENT_TIMESTAMP)
ON CONFLICT(chat_id) DO UPDATE SET
  current_pane_key = excluded.current_pane_key,
  updated_at = CURRENT_TIMESTAMP;
`, chatID, sqlString(strings.TrimSpace(paneKey)))
	return s.exec(ctx, query)
}

func (s *Store) CurrentPane(ctx context.Context, chatID int64) (string, error) {
	type row struct {
		CurrentPaneKey string `json:"current_pane_key"`
	}
	var rows []row
	query := fmt.Sprintf(`
SELECT current_pane_key
FROM chat_state
WHERE chat_id = %d
LIMIT 1;
`, chatID)
	if err := s.queryJSON(ctx, query, &rows); err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	return strings.TrimSpace(rows[0].CurrentPaneKey), nil
}

func (s *Store) ListChats(ctx context.Context) ([]int64, error) {
	type row struct {
		ChatID json.Number `json:"chat_id"`
	}
	var rows []row
	query := `
SELECT chat_id
FROM (
  SELECT chat_id FROM chat_state
  UNION
  SELECT chat_id FROM chat_bindings
)
ORDER BY chat_id;
`
	if err := s.queryJSON(ctx, query, &rows); err != nil {
		return nil, err
	}
	result := make([]int64, 0, len(rows))
	for _, row := range rows {
		value, err := row.ChatID.Int64()
		if err != nil {
			return nil, fmt.Errorf("parse chat id: %w", err)
		}
		result = append(result, value)
	}
	return result, nil
}

func (s *Store) LogMessage(ctx context.Context, record MessageRecord) error {
	query := fmt.Sprintf(`
INSERT INTO message_log (
  chat_id,
  pane_key,
  direction,
  kind,
  telegram_message_id,
  body_preview
)
VALUES (%d, %s, %s, %s, %d, %s);
`, record.ChatID, sqlString(strings.TrimSpace(record.PaneKey)), sqlString(strings.TrimSpace(record.Direction)), sqlString(strings.TrimSpace(record.Kind)), record.TelegramMessageID, sqlString(truncatePreview(record.BodyPreview)))
	return s.exec(ctx, query)
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
     SELECT chat_id FROM chat_state
     UNION
     SELECT chat_id FROM chat_bindings
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

func (s *Store) exec(ctx context.Context, query string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cmd := exec.CommandContext(ctx, s.sqliteBin, s.path, query)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sqlite3 exec failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (s *Store) queryJSON(ctx context.Context, query string, dest any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cmd := exec.CommandContext(ctx, s.sqliteBin, "-json", s.path, query)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sqlite3 query failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if len(strings.TrimSpace(string(output))) == 0 {
		output = []byte("[]")
	}
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	decoder.UseNumber()
	if err := decoder.Decode(dest); err != nil {
		return fmt.Errorf("decode sqlite json: %w", err)
	}
	return nil
}

func sqlString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func truncatePreview(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= 240 {
		return value
	}
	return string(runes[:240]) + "..."
}

func numberToInt(value json.Number) (int, error) {
	parsed, err := strconv.ParseInt(string(value), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse sqlite count %q: %w", string(value), err)
	}
	return int(parsed), nil
}

func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join(".", ".tagb", "tagb.db")
	}
	return filepath.Join(home, ".tagb", "tagb.db")
}
