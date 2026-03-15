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

type SessionRecord struct {
	SessionKey            string
	Platform              string
	ChatID                int64
	PaneKey               string
	Agent                 string
	AgentSessionID        string
	AgentThreadID         string
	LastInboundMessageID  int64
	LastOutboundMessageID int64
}

type MessageLinkRecord struct {
	Platform          string
	ChatID            int64
	PaneKey           string
	SessionKey        string
	Kind              string
	InboundMessageID  int64
	OutboundMessageID int64
	ReplyToMessageID  int64
}

type StoreStats struct {
	Chats    int
	Bindings int
	Messages int
}

const (
	schemaVersionPhase2 = 1
	schemaVersionPhase3 = 2
)

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
	if err := s.exec(ctx, schema); err != nil {
		return err
	}
	return s.applyMigrations(ctx)
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

func (s *Store) EnsureSession(ctx context.Context, platform string, chatID int64, paneKey string, agent string) (SessionRecord, error) {
	platform = strings.TrimSpace(platform)
	paneKey = strings.TrimSpace(paneKey)
	agent = strings.TrimSpace(agent)
	if platform == "" {
		return SessionRecord{}, fmt.Errorf("platform is required")
	}
	if paneKey == "" {
		return SessionRecord{}, fmt.Errorf("pane key is required")
	}
	sessionKey := sessionKeyFor(platform, chatID, paneKey)
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
VALUES (%s, %s, %d, %s, %s, '', '', 0, 0)
ON CONFLICT(session_key) DO UPDATE SET
  pane_key = excluded.pane_key,
  agent = CASE
    WHEN trim(excluded.agent) <> '' THEN excluded.agent
    ELSE sessions.agent
  END,
  updated_at = CURRENT_TIMESTAMP
RETURNING session_key, platform, chat_id, pane_key, agent, agent_session_id, agent_thread_id, last_inbound_message_id, last_outbound_message_id;
`, sqlString(sessionKey), sqlString(platform), chatID, sqlString(paneKey), sqlString(agent))
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
VALUES (%s, %s, %d, %s, %s, '', '', 0, 0)
ON CONFLICT(session_key) DO UPDATE SET
  pane_key = excluded.pane_key,
  agent = CASE
    WHEN trim(excluded.agent) <> '' THEN excluded.agent
    ELSE sessions.agent
  END,
  updated_at = CURRENT_TIMESTAMP;
`, sqlString(sessionKey), sqlString(platform), chatID, sqlString(paneKey), sqlString(agent))
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

func (s *Store) LatestSessionByChatPane(ctx context.Context, platform string, chatID int64, paneKey string) (SessionRecord, error) {
	var rows []sessionRow
	query := fmt.Sprintf(`
SELECT session_key, platform, chat_id, pane_key, agent, agent_session_id, agent_thread_id, last_inbound_message_id, last_outbound_message_id
FROM sessions
WHERE platform = %s AND chat_id = %d AND pane_key = %s
ORDER BY updated_at DESC
LIMIT 1;
`, sqlString(strings.TrimSpace(platform)), chatID, sqlString(strings.TrimSpace(paneKey)))
	if err := s.queryJSON(ctx, query, &rows); err != nil {
		return SessionRecord{}, err
	}
	if len(rows) == 0 {
		return SessionRecord{}, nil
	}
	return rows[0].toRecord()
}

func (s *Store) TouchSessionInbound(ctx context.Context, sessionKey string, messageID int64) error {
	query := fmt.Sprintf(`
UPDATE sessions
SET last_inbound_message_id = %d,
    updated_at = CURRENT_TIMESTAMP
WHERE session_key = %s;
`, messageID, sqlString(strings.TrimSpace(sessionKey)))
	return s.exec(ctx, query)
}

func (s *Store) TouchSessionOutbound(ctx context.Context, sessionKey string, messageID int64) error {
	query := fmt.Sprintf(`
UPDATE sessions
SET last_outbound_message_id = %d,
    updated_at = CURRENT_TIMESTAMP
WHERE session_key = %s;
`, messageID, sqlString(strings.TrimSpace(sessionKey)))
	return s.exec(ctx, query)
}

func (s *Store) CreateMessageLink(ctx context.Context, record MessageLinkRecord) error {
	query := fmt.Sprintf(`
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
VALUES (%s, %d, %s, %s, %s, %d, %d, %d);
`, sqlString(strings.TrimSpace(record.Platform)), record.ChatID, sqlString(strings.TrimSpace(record.PaneKey)), sqlString(strings.TrimSpace(record.SessionKey)), sqlString(strings.TrimSpace(record.Kind)), record.InboundMessageID, record.OutboundMessageID, record.ReplyToMessageID)
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

func (s *Store) applyMigrations(ctx context.Context) error {
	version, err := s.schemaVersion(ctx)
	if err != nil {
		return err
	}
	if version < schemaVersionPhase2 {
		if err := s.setSchemaVersion(ctx, schemaVersionPhase2); err != nil {
			return err
		}
		version = schemaVersionPhase2
	}
	if version < schemaVersionPhase3 {
		query := fmt.Sprintf(`
BEGIN;
CREATE TABLE IF NOT EXISTS sessions (
  session_key TEXT PRIMARY KEY,
  platform TEXT NOT NULL,
  chat_id INTEGER NOT NULL,
  pane_key TEXT NOT NULL,
  agent TEXT NOT NULL DEFAULT '',
  agent_session_id TEXT NOT NULL DEFAULT '',
  agent_thread_id TEXT NOT NULL DEFAULT '',
  last_inbound_message_id INTEGER NOT NULL DEFAULT 0,
  last_outbound_message_id INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(platform, chat_id, pane_key)
);
CREATE INDEX IF NOT EXISTS idx_sessions_chat_updated_at
  ON sessions (chat_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_pane_updated_at
  ON sessions (pane_key, updated_at DESC);
CREATE TABLE IF NOT EXISTS message_links (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  platform TEXT NOT NULL,
  chat_id INTEGER NOT NULL,
  pane_key TEXT NOT NULL DEFAULT '',
  session_key TEXT NOT NULL,
  kind TEXT NOT NULL,
  inbound_message_id INTEGER NOT NULL DEFAULT 0,
  outbound_message_id INTEGER NOT NULL DEFAULT 0,
  reply_to_message_id INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_message_links_session_created_at
  ON message_links (session_key, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_message_links_chat_pane_created_at
  ON message_links (chat_id, pane_key, created_at DESC);
PRAGMA user_version = %d;
COMMIT;
`, schemaVersionPhase3)
		if err := s.exec(ctx, query); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) schemaVersion(ctx context.Context) (int, error) {
	type row struct {
		UserVersion json.Number `json:"user_version"`
	}
	var rows []row
	if err := s.queryJSON(ctx, "PRAGMA user_version;", &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return numberToInt(rows[0].UserVersion)
}

func (s *Store) setSchemaVersion(ctx context.Context, version int) error {
	return s.exec(ctx, fmt.Sprintf("PRAGMA user_version = %d;", version))
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

func parseJSONInt64(value json.Number) (int64, error) {
	if string(value) == "" {
		return 0, nil
	}
	parsed, err := value.Int64()
	if err != nil {
		return 0, fmt.Errorf("parse sqlite int64 %q: %w", string(value), err)
	}
	return parsed, nil
}

type sessionRow struct {
	SessionKey            string      `json:"session_key"`
	Platform              string      `json:"platform"`
	ChatID                json.Number `json:"chat_id"`
	PaneKey               string      `json:"pane_key"`
	Agent                 string      `json:"agent"`
	AgentSessionID        string      `json:"agent_session_id"`
	AgentThreadID         string      `json:"agent_thread_id"`
	LastInboundMessageID  json.Number `json:"last_inbound_message_id"`
	LastOutboundMessageID json.Number `json:"last_outbound_message_id"`
}

func (r sessionRow) toRecord() (SessionRecord, error) {
	chatID, err := r.ChatID.Int64()
	if err != nil {
		return SessionRecord{}, fmt.Errorf("parse chat id: %w", err)
	}
	lastInbound, err := parseJSONInt64(r.LastInboundMessageID)
	if err != nil {
		return SessionRecord{}, err
	}
	lastOutbound, err := parseJSONInt64(r.LastOutboundMessageID)
	if err != nil {
		return SessionRecord{}, err
	}
	return SessionRecord{
		SessionKey:            r.SessionKey,
		Platform:              r.Platform,
		ChatID:                chatID,
		PaneKey:               r.PaneKey,
		Agent:                 r.Agent,
		AgentSessionID:        r.AgentSessionID,
		AgentThreadID:         r.AgentThreadID,
		LastInboundMessageID:  lastInbound,
		LastOutboundMessageID: lastOutbound,
	}, nil
}

func sessionKeyFor(platform string, chatID int64, paneKey string) string {
	return strings.TrimSpace(platform) + ":" + strconv.FormatInt(chatID, 10) + ":" + strings.TrimSpace(paneKey)
}

func isReturningUnsupported(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "RETURNING") && strings.Contains(strings.ToLower(message), "syntax error")
}

func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join(".", ".tagb", "tagb.db")
	}
	return filepath.Join(home, ".tagb", "tagb.db")
}
