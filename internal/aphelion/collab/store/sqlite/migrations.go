package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
)

const schemaVersion = 3

//go:embed schema/001_initial.sql
var initialSchema string

//go:embed schema/002_revision_hashes.sql
var revisionHashesSchema string

//go:embed schema/003_client_sessions.sql
var clientSessionsSchema string

func migrate(ctx context.Context, database *sql.DB) error {
	var version int
	if err := database.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version > schemaVersion {
		return fmt.Errorf("database schema version is %d, maximum supported is %d", version, schemaVersion)
	}
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin schema migration: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()
	if version < 1 {
		if _, err := transaction.ExecContext(ctx, initialSchema); err != nil {
			return fmt.Errorf("apply initial schema: %w", err)
		}
	}
	if version < 2 {
		if _, err := transaction.ExecContext(ctx, revisionHashesSchema); err != nil {
			return fmt.Errorf("apply revision hashes schema: %w", err)
		}
	}
	if version < 3 {
		if _, err := transaction.ExecContext(ctx, clientSessionsSchema); err != nil {
			return fmt.Errorf("apply client sessions schema: %w", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit schema migration: %w", err)
	}
	return nil
}
