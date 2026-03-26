package daemon

import (
	"context"
	"fmt"
)

const (
	schemaVersionPhase2 = 1
	schemaVersionPhase3 = 2
	schemaVersionPhase4 = 3
	schemaVersionPhase5 = 4
	schemaVersionPhase6 = 5
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
		version = schemaVersionPhase5
	}
	if version < schemaVersionPhase6 {
		query := phase6MigrationSQL()
		if err := s.exec(ctx, query); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) schemaVersion(ctx context.Context) (int, error) {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version;").Scan(&version); err != nil {
		return 0, fmt.Errorf("sqlite query failed: %w", err)
	}
	return version, nil
}

func (s *Store) setSchemaVersion(ctx context.Context, version int) error {
	return s.exec(ctx, fmt.Sprintf("PRAGMA user_version = %d;", version))
}
