package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func (s *Store) GetPlatformRuntimeState(ctx context.Context, platform string, scope string, entityID string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `
SELECT value
FROM platform_runtime_state
WHERE platform = ? AND scope = ? AND entity_id = ?
LIMIT 1;
`, normalizePlatformName(platform), strings.TrimSpace(scope), strings.TrimSpace(entityID)).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("sqlite query failed: %w", err)
	}
	return value, nil
}

func (s *Store) SetPlatformRuntimeState(ctx context.Context, platform string, scope string, entityID string, value string) error {
	return s.execArgs(ctx, `
INSERT INTO platform_runtime_state (platform, scope, entity_id, value, updated_at)
VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(platform, scope, entity_id) DO UPDATE SET
  value = excluded.value,
  updated_at = CURRENT_TIMESTAMP;
`, normalizePlatformName(platform), strings.TrimSpace(scope), strings.TrimSpace(entityID), value)
}
