package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	path string
	db   *sql.DB
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

func OpenStore(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("db path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{
		path: path,
		db:   db,
	}
	if err := store.configure(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.init(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) configure(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite db: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
`); err != nil {
		return fmt.Errorf("configure sqlite db: %w", err)
	}
	return nil
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
`
	if err := s.exec(ctx, schema); err != nil {
		return err
	}
	return s.applyMigrations(ctx)
}
