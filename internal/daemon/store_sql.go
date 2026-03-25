package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s *Store) exec(ctx context.Context, query string) error {
	if _, err := s.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("sqlite exec failed: %w", err)
	}
	return nil
}

func (s *Store) execArgs(ctx context.Context, query string, args ...any) error {
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("sqlite exec failed: %w", err)
	}
	return nil
}

func (s *Store) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite begin tx: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite commit tx: %w", err)
	}
	return nil
}

func (s *Store) tableHasColumn(ctx context.Context, table string, column string) (bool, error) {
	query := fmt.Sprintf("PRAGMA table_info(%s);", sqlIdent(table))
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return false, fmt.Errorf("sqlite query failed: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid      int
			name     string
			typeName string
			notNull  int
			defaultV any
			pk       int
		)
		if err := rows.Scan(&cid, &name, &typeName, &notNull, &defaultV, &pk); err != nil {
			return false, fmt.Errorf("sqlite scan: %w", err)
		}
		if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(column)) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("sqlite rows: %w", err)
	}
	return false, nil
}

func sqlIdent(value string) string {
	return `"` + strings.ReplaceAll(strings.TrimSpace(value), `"`, `""`) + `"`
}

func truncatePreview(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= 240 {
		return value
	}
	return string(runes[:240]) + "..."
}

func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join(".", ".tmuxconn", "tmuxconn.db")
	}
	return filepath.Join(home, ".tmuxconn", "tmuxconn.db")
}
