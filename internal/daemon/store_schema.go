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
		query := phase3MigrationSQL()
		if err := s.exec(ctx, query); err != nil {
			return err
		}
		version = schemaVersionPhase3
	}
	if version < schemaVersionPhase4 {
		plan, err := s.phase4MigrationPlan(ctx)
		if err != nil {
			return err
		}
		query := phase4MigrationSQL(plan)
		if err := s.exec(ctx, query); err != nil {
			return err
		}
		version = schemaVersionPhase4
	}
	if version < schemaVersionPhase5 {
		query := phase5MigrationSQL()
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
