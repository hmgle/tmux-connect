package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func (s *Store) exec(ctx context.Context, query string) error {
	if _, err := s.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("sqlite exec failed: %w", err)
	}
	return nil
}

func (s *Store) queryJSON(ctx context.Context, query string, dest any) error {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("sqlite query failed: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("sqlite columns: %w", err)
	}

	records := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		scanTargets := make([]any, len(columns))
		for i := range values {
			scanTargets[i] = &values[i]
		}
		if err := rows.Scan(scanTargets...); err != nil {
			return fmt.Errorf("sqlite scan: %w", err)
		}

		record := make(map[string]any, len(columns))
		for i, column := range columns {
			record[column] = normalizeSQLiteValue(values[i])
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite rows: %w", err)
	}

	output, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("marshal sqlite rows: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.UseNumber()
	if err := decoder.Decode(dest); err != nil {
		return fmt.Errorf("decode sqlite json: %w", err)
	}
	return nil
}

func normalizeSQLiteValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	default:
		return typed
	}
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

func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join(".", ".tmuxconn", "tmuxconn.db")
	}
	return filepath.Join(home, ".tmuxconn", "tmuxconn.db")
}
