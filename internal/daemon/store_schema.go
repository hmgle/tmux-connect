package daemon

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	schemaVersionPhase2 = 1
	schemaVersionPhase3 = 2
	schemaVersionPhase4 = 3
	schemaVersionPhase5 = 4
)

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
