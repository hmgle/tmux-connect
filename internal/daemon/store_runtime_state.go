package daemon

import (
	"context"
	"fmt"
	"strings"
)

func (s *Store) GetPlatformRuntimeState(ctx context.Context, platform string, scope string, entityID string) (string, error) {
	type row struct {
		Value string `json:"value"`
	}
	var rows []row
	query := fmt.Sprintf(`
SELECT value
FROM platform_runtime_state
WHERE platform = %s AND scope = %s AND entity_id = %s
LIMIT 1;
`, sqlString(normalizePlatformName(platform)), sqlString(strings.TrimSpace(scope)), sqlString(strings.TrimSpace(entityID)))
	if err := s.queryJSON(ctx, query, &rows); err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	return rows[0].Value, nil
}

func (s *Store) SetPlatformRuntimeState(ctx context.Context, platform string, scope string, entityID string, value string) error {
	query := fmt.Sprintf(`
INSERT INTO platform_runtime_state (platform, scope, entity_id, value, updated_at)
VALUES (%s, %s, %s, %s, CURRENT_TIMESTAMP)
ON CONFLICT(platform, scope, entity_id) DO UPDATE SET
  value = excluded.value,
  updated_at = CURRENT_TIMESTAMP;
`, sqlString(normalizePlatformName(platform)), sqlString(strings.TrimSpace(scope)), sqlString(strings.TrimSpace(entityID)), sqlString(value))
	return s.exec(ctx, query)
}
