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
	Chat              ChatRef
	PaneKey           string
	Direction         string
	Kind              string
	PlatformMessageID string
	ThreadID          string
	BodyPreview       string
}

type SessionRecord struct {
	SessionKey            string
	Platform              string
	ChatID                string
	PaneKey               string
	Agent                 string
	AgentSessionID        string
	AgentThreadID         string
	LastInboundMessageID  string
	LastOutboundMessageID string
}

type MessageLinkRecord struct {
	Platform          string
	ChatID            string
	PaneKey           string
	SessionKey        string
	Kind              string
	InboundMessageID  string
	OutboundMessageID string
	ReplyToMessageID  string
}

type StoreStats struct {
	Chats    int
	Bindings int
	Messages int
}

const (
	schemaVersionPhase2 = 1
	schemaVersionPhase3 = 2
	schemaVersionPhase4 = 3
	schemaVersionPhase5 = 4
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
  platform TEXT NOT NULL,
  chat_id TEXT NOT NULL,
  pane_key TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (platform, chat_id, pane_key)
);
CREATE TABLE IF NOT EXISTS chat_state (
  platform TEXT NOT NULL,
  chat_id TEXT NOT NULL,
  current_pane_key TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (platform, chat_id)
);
CREATE TABLE IF NOT EXISTS message_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  platform TEXT NOT NULL,
  chat_id TEXT NOT NULL,
  pane_key TEXT NOT NULL DEFAULT '',
  direction TEXT NOT NULL,
  kind TEXT NOT NULL,
  platform_message_id TEXT NOT NULL DEFAULT '',
  thread_id TEXT NOT NULL DEFAULT '',
  body_preview TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_message_log_platform_chat_created_at
  ON message_log (platform, chat_id, created_at DESC);
`
	if err := s.exec(ctx, schema); err != nil {
		return err
	}
	return s.applyMigrations(ctx)
}

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

func logMessageStatement(record MessageRecord) string {
	chat := record.Chat.Normalized()
	return fmt.Sprintf(`
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
VALUES (%s, %s, %s, %s, %s, %s, %s, %s);
`, sqlString(chat.Platform), sqlString(chat.ChatID), sqlString(strings.TrimSpace(record.PaneKey)), sqlString(strings.TrimSpace(record.Direction)), sqlString(strings.TrimSpace(record.Kind)), sqlString(strings.TrimSpace(record.PlatformMessageID)), sqlString(strings.TrimSpace(record.ThreadID)), sqlString(truncatePreview(record.BodyPreview)))
}

func (s *Store) EnsureSession(ctx context.Context, chat ChatRef, paneKey string, agent string) (SessionRecord, error) {
	chat = chat.Normalized()
	paneKey = strings.TrimSpace(paneKey)
	agent = strings.TrimSpace(agent)
	if !chat.Valid() {
		return SessionRecord{}, fmt.Errorf("chat ref is required")
	}
	if paneKey == "" {
		return SessionRecord{}, fmt.Errorf("pane key is required")
	}
	sessionKey := sessionKeyFor(chat, paneKey)
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
VALUES (%s, %s, %s, %s, %s, '', '', '', '')
ON CONFLICT(session_key) DO UPDATE SET
  pane_key = excluded.pane_key,
  agent = CASE
    WHEN trim(excluded.agent) <> '' THEN excluded.agent
    ELSE sessions.agent
  END,
  updated_at = CURRENT_TIMESTAMP
RETURNING session_key, platform, chat_id, pane_key, agent, agent_session_id, agent_thread_id, last_inbound_message_id, last_outbound_message_id;
`, sqlString(sessionKey), sqlString(chat.Platform), sqlString(chat.ChatID), sqlString(paneKey), sqlString(agent))
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
VALUES (%s, %s, %s, %s, %s, '', '', '', '')
ON CONFLICT(session_key) DO UPDATE SET
  pane_key = excluded.pane_key,
  agent = CASE
    WHEN trim(excluded.agent) <> '' THEN excluded.agent
    ELSE sessions.agent
  END,
  updated_at = CURRENT_TIMESTAMP;
`, sqlString(sessionKey), sqlString(chat.Platform), sqlString(chat.ChatID), sqlString(paneKey), sqlString(agent))
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

func (s *Store) LatestSessionByChatPane(ctx context.Context, chat ChatRef, paneKey string) (SessionRecord, error) {
	chat = chat.Normalized()
	var rows []sessionRow
	query := fmt.Sprintf(`
SELECT session_key, platform, chat_id, pane_key, agent, agent_session_id, agent_thread_id, last_inbound_message_id, last_outbound_message_id
FROM sessions
WHERE platform = %s AND chat_id = %s AND pane_key = %s
ORDER BY updated_at DESC
LIMIT 1;
`, sqlString(chat.Platform), sqlString(chat.ChatID), sqlString(strings.TrimSpace(paneKey)))
	if err := s.queryJSON(ctx, query, &rows); err != nil {
		return SessionRecord{}, err
	}
	if len(rows) == 0 {
		return SessionRecord{}, nil
	}
	return rows[0].toRecord()
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

func (s *Store) TouchSessionInbound(ctx context.Context, sessionKey string, messageID string) error {
	return s.exec(ctx, touchSessionInboundStatement(sessionKey, messageID))
}

func touchSessionInboundStatement(sessionKey string, messageID string) string {
	return fmt.Sprintf(`
UPDATE sessions
SET last_inbound_message_id = %s,
    updated_at = CURRENT_TIMESTAMP
WHERE session_key = %s;
`, sqlString(strings.TrimSpace(messageID)), sqlString(strings.TrimSpace(sessionKey)))
}

func (s *Store) TouchSessionOutbound(ctx context.Context, sessionKey string, messageID string) error {
	return s.exec(ctx, touchSessionOutboundStatement(sessionKey, messageID))
}

func touchSessionOutboundStatement(sessionKey string, messageID string) string {
	return fmt.Sprintf(`
UPDATE sessions
SET last_outbound_message_id = %s,
    updated_at = CURRENT_TIMESTAMP
WHERE session_key = %s;
`, sqlString(strings.TrimSpace(messageID)), sqlString(strings.TrimSpace(sessionKey)))
}

func updateSessionThreadStatement(sessionKey string, threadID string) string {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return ""
	}
	return fmt.Sprintf(`
UPDATE sessions
SET agent_thread_id = %s,
    updated_at = CURRENT_TIMESTAMP
WHERE session_key = %s;
`, sqlString(threadID), sqlString(strings.TrimSpace(sessionKey)))
}

func (s *Store) CreateMessageLink(ctx context.Context, record MessageLinkRecord) error {
	return s.exec(ctx, createMessageLinkStatement(record))
}

func createMessageLinkStatement(record MessageLinkRecord) string {
	return fmt.Sprintf(`
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
VALUES (%s, %s, %s, %s, %s, %s, %s, %s);
`, sqlString(strings.TrimSpace(record.Platform)), sqlString(strings.TrimSpace(record.ChatID)), sqlString(strings.TrimSpace(record.PaneKey)), sqlString(strings.TrimSpace(record.SessionKey)), sqlString(strings.TrimSpace(record.Kind)), sqlString(strings.TrimSpace(record.InboundMessageID)), sqlString(strings.TrimSpace(record.OutboundMessageID)), sqlString(strings.TrimSpace(record.ReplyToMessageID)))
}

func ensureSessionStatement(chat ChatRef, paneKey string, agent string, sessionKey string) string {
	chat = chat.Normalized()
	return fmt.Sprintf(`
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
VALUES (%s, %s, %s, %s, %s, '', '', '', '')
ON CONFLICT(session_key) DO UPDATE SET
  pane_key = excluded.pane_key,
  agent = CASE
    WHEN trim(excluded.agent) <> '' THEN excluded.agent
    ELSE sessions.agent
  END,
  updated_at = CURRENT_TIMESTAMP;
`, sqlString(strings.TrimSpace(sessionKey)), sqlString(chat.Platform), sqlString(chat.ChatID), sqlString(strings.TrimSpace(paneKey)), sqlString(strings.TrimSpace(agent)))
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
  chat_id TEXT NOT NULL,
  pane_key TEXT NOT NULL,
  agent TEXT NOT NULL DEFAULT '',
  agent_session_id TEXT NOT NULL DEFAULT '',
  agent_thread_id TEXT NOT NULL DEFAULT '',
  last_inbound_message_id TEXT NOT NULL DEFAULT '',
  last_outbound_message_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(platform, chat_id, pane_key)
);
CREATE INDEX IF NOT EXISTS idx_sessions_platform_chat_updated_at
  ON sessions (platform, chat_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_pane_updated_at
  ON sessions (pane_key, updated_at DESC);
CREATE TABLE IF NOT EXISTS message_links (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  platform TEXT NOT NULL,
  chat_id TEXT NOT NULL,
  pane_key TEXT NOT NULL DEFAULT '',
  session_key TEXT NOT NULL,
  kind TEXT NOT NULL,
  inbound_message_id TEXT NOT NULL DEFAULT '',
  outbound_message_id TEXT NOT NULL DEFAULT '',
  reply_to_message_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_message_links_session_created_at
  ON message_links (session_key, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_message_links_platform_chat_pane_created_at
  ON message_links (platform, chat_id, pane_key, created_at DESC);
PRAGMA user_version = %d;
COMMIT;
`, schemaVersionPhase3)
		if err := s.exec(ctx, query); err != nil {
			return err
		}
		version = schemaVersionPhase3
	}
	if version < schemaVersionPhase4 {
		hasBindingsPlatform, err := s.tableHasColumn(ctx, "chat_bindings", "platform")
		if err != nil {
			return err
		}
		hasStatePlatform, err := s.tableHasColumn(ctx, "chat_state", "platform")
		if err != nil {
			return err
		}
		hasMessageLogPlatform, err := s.tableHasColumn(ctx, "message_log", "platform")
		if err != nil {
			return err
		}
		hasPlatformMessageID, err := s.tableHasColumn(ctx, "message_log", "platform_message_id")
		if err != nil {
			return err
		}
		hasTelegramMessageID, err := s.tableHasColumn(ctx, "message_log", "telegram_message_id")
		if err != nil {
			return err
		}
		hasThreadID, err := s.tableHasColumn(ctx, "message_log", "thread_id")
		if err != nil {
			return err
		}

		bindingsPlatformExpr := "'telegram'"
		if hasBindingsPlatform {
			bindingsPlatformExpr = "COALESCE(NULLIF(CAST(platform AS TEXT), ''), 'telegram')"
		}
		statePlatformExpr := "'telegram'"
		if hasStatePlatform {
			statePlatformExpr = "COALESCE(NULLIF(CAST(platform AS TEXT), ''), 'telegram')"
		}
		messageLogPlatformExpr := "'telegram'"
		if hasMessageLogPlatform {
			messageLogPlatformExpr = "COALESCE(NULLIF(CAST(platform AS TEXT), ''), 'telegram')"
		}
		messageIDExpr := "''"
		switch {
		case hasPlatformMessageID:
			messageIDExpr = "COALESCE(CAST(platform_message_id AS TEXT), '')"
		case hasTelegramMessageID:
			messageIDExpr = "CASE WHEN telegram_message_id = 0 THEN '' ELSE CAST(telegram_message_id AS TEXT) END"
		}
		threadIDExpr := "''"
		if hasThreadID {
			threadIDExpr = "COALESCE(CAST(thread_id AS TEXT), '')"
		}

		query := fmt.Sprintf(`
BEGIN;
ALTER TABLE chat_bindings RENAME TO chat_bindings_old;
CREATE TABLE chat_bindings (
  platform TEXT NOT NULL,
  chat_id TEXT NOT NULL,
  pane_key TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (platform, chat_id, pane_key)
);
INSERT INTO chat_bindings (platform, chat_id, pane_key, created_at)
SELECT
  %s,
  CAST(chat_id AS TEXT),
  pane_key,
  created_at
FROM chat_bindings_old;
DROP TABLE chat_bindings_old;

ALTER TABLE chat_state RENAME TO chat_state_old;
CREATE TABLE chat_state (
  platform TEXT NOT NULL,
  chat_id TEXT NOT NULL,
  current_pane_key TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (platform, chat_id)
);
INSERT INTO chat_state (platform, chat_id, current_pane_key, updated_at)
SELECT
  %s,
  CAST(chat_id AS TEXT),
  current_pane_key,
  updated_at
FROM chat_state_old;
DROP TABLE chat_state_old;

ALTER TABLE message_log RENAME TO message_log_old;
CREATE TABLE message_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  platform TEXT NOT NULL,
  chat_id TEXT NOT NULL,
  pane_key TEXT NOT NULL DEFAULT '',
  direction TEXT NOT NULL,
  kind TEXT NOT NULL,
  platform_message_id TEXT NOT NULL DEFAULT '',
  thread_id TEXT NOT NULL DEFAULT '',
  body_preview TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO message_log (id, platform, chat_id, pane_key, direction, kind, platform_message_id, thread_id, body_preview, created_at)
SELECT
  id,
  %s,
  CAST(chat_id AS TEXT),
  pane_key,
  direction,
  kind,
  %s,
  %s,
  body_preview,
  created_at
FROM message_log_old;
DROP TABLE message_log_old;
CREATE INDEX IF NOT EXISTS idx_message_log_platform_chat_created_at
  ON message_log (platform, chat_id, created_at DESC);

ALTER TABLE sessions RENAME TO sessions_old;
CREATE TABLE sessions (
  session_key TEXT PRIMARY KEY,
  platform TEXT NOT NULL,
  chat_id TEXT NOT NULL,
  pane_key TEXT NOT NULL,
  agent TEXT NOT NULL DEFAULT '',
  agent_session_id TEXT NOT NULL DEFAULT '',
  agent_thread_id TEXT NOT NULL DEFAULT '',
  last_inbound_message_id TEXT NOT NULL DEFAULT '',
  last_outbound_message_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(platform, chat_id, pane_key)
);
INSERT INTO sessions (session_key, platform, chat_id, pane_key, agent, agent_session_id, agent_thread_id, last_inbound_message_id, last_outbound_message_id, created_at, updated_at)
SELECT
  session_key,
  platform,
  CAST(chat_id AS TEXT),
  pane_key,
  agent,
  agent_session_id,
  agent_thread_id,
  CASE WHEN last_inbound_message_id = 0 THEN '' ELSE CAST(last_inbound_message_id AS TEXT) END,
  CASE WHEN last_outbound_message_id = 0 THEN '' ELSE CAST(last_outbound_message_id AS TEXT) END,
  created_at,
  updated_at
FROM sessions_old;
DROP TABLE sessions_old;
CREATE INDEX IF NOT EXISTS idx_sessions_platform_chat_updated_at
  ON sessions (platform, chat_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_pane_updated_at
  ON sessions (pane_key, updated_at DESC);

ALTER TABLE message_links RENAME TO message_links_old;
CREATE TABLE message_links (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  platform TEXT NOT NULL,
  chat_id TEXT NOT NULL,
  pane_key TEXT NOT NULL DEFAULT '',
  session_key TEXT NOT NULL,
  kind TEXT NOT NULL,
  inbound_message_id TEXT NOT NULL DEFAULT '',
  outbound_message_id TEXT NOT NULL DEFAULT '',
  reply_to_message_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO message_links (id, platform, chat_id, pane_key, session_key, kind, inbound_message_id, outbound_message_id, reply_to_message_id, created_at)
SELECT
  id,
  platform,
  CAST(chat_id AS TEXT),
  pane_key,
  session_key,
  kind,
  CASE WHEN inbound_message_id = 0 THEN '' ELSE CAST(inbound_message_id AS TEXT) END,
  CASE WHEN outbound_message_id = 0 THEN '' ELSE CAST(outbound_message_id AS TEXT) END,
  CASE WHEN reply_to_message_id = 0 THEN '' ELSE CAST(reply_to_message_id AS TEXT) END,
  created_at
FROM message_links_old;
DROP TABLE message_links_old;
CREATE INDEX IF NOT EXISTS idx_message_links_session_created_at
  ON message_links (session_key, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_message_links_platform_chat_pane_created_at
  ON message_links (platform, chat_id, pane_key, created_at DESC);
PRAGMA user_version = %d;
COMMIT;
`, bindingsPlatformExpr, statePlatformExpr, messageLogPlatformExpr, messageIDExpr, threadIDExpr, schemaVersionPhase4)
		if err := s.exec(ctx, query); err != nil {
			return err
		}
		version = schemaVersionPhase4
	}
	if version < schemaVersionPhase5 {
		query := fmt.Sprintf(`
BEGIN;
CREATE INDEX IF NOT EXISTS idx_message_log_platform_chat_thread_created_at
  ON message_log (platform, chat_id, thread_id, created_at DESC);
PRAGMA user_version = %d;
COMMIT;
`, schemaVersionPhase5)
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

func (s *Store) tableHasColumn(ctx context.Context, table string, column string) (bool, error) {
	type row struct {
		Name string `json:"name"`
	}
	var rows []row
	query := fmt.Sprintf("PRAGMA table_info(%s);", sqlIdent(table))
	if err := s.queryJSON(ctx, query, &rows); err != nil {
		return false, err
	}
	for _, row := range rows {
		if strings.EqualFold(strings.TrimSpace(row.Name), strings.TrimSpace(column)) {
			return true, nil
		}
	}
	return false, nil
}

func sqlString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func sqlIdent(value string) string {
	return `"` + strings.ReplaceAll(strings.TrimSpace(value), `"`, `""`) + `"`
}

func wrapTransaction(statements ...string) string {
	if len(statements) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("BEGIN;\n")
	for _, statement := range statements {
		trimmed := strings.TrimSpace(statement)
		if trimmed == "" {
			continue
		}
		b.WriteString(trimmed)
		if !strings.HasSuffix(trimmed, ";") {
			b.WriteString(";")
		}
		b.WriteString("\n")
	}
	b.WriteString("COMMIT;\n")
	return b.String()
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
		return filepath.Join(".", ".tmuxconn", "tmuxconn.db")
	}
	return filepath.Join(home, ".tmuxconn", "tmuxconn.db")
}
