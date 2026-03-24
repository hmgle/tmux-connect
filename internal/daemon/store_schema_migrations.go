package daemon

import "fmt"

func phase3MigrationSQL() string {
	return fmt.Sprintf(`
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
}

func phase5MigrationSQL() string {
	return fmt.Sprintf(`
BEGIN;
CREATE INDEX IF NOT EXISTS idx_message_log_platform_chat_thread_created_at
  ON message_log (platform, chat_id, thread_id, created_at DESC);
PRAGMA user_version = %d;
COMMIT;
`, schemaVersionPhase5)
}

func phase6MigrationSQL() string {
	return fmt.Sprintf(`
BEGIN;
CREATE TABLE IF NOT EXISTS platform_runtime_state (
  platform TEXT NOT NULL,
  scope TEXT NOT NULL,
  entity_id TEXT NOT NULL DEFAULT '',
  value TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (platform, scope, entity_id)
);
CREATE INDEX IF NOT EXISTS idx_platform_runtime_state_platform_scope_updated_at
  ON platform_runtime_state (platform, scope, updated_at DESC);
PRAGMA user_version = %d;
COMMIT;
`, schemaVersionPhase6)
}
