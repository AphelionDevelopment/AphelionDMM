package postgres

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	postgresSchemaVersion = 1
	migrationLockID       = 0x415048454c494f4e
)

//go:embed schema/001_initial.sql
var initialSchema string

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	transaction, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin PostgreSQL migration: %w", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()
	if _, err := transaction.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(migrationLockID)); err != nil {
		return fmt.Errorf("lock PostgreSQL migrations: %w", err)
	}
	if _, err := transaction.Exec(ctx, `CREATE TABLE IF NOT EXISTS collaboration_schema_migrations (version INTEGER PRIMARY KEY CHECK (version > 0), applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		return fmt.Errorf("create PostgreSQL migration table: %w", err)
	}
	var version int
	if err := transaction.QueryRow(ctx, "SELECT COALESCE(MAX(version), 0) FROM collaboration_schema_migrations").Scan(&version); err != nil {
		return fmt.Errorf("read PostgreSQL schema version: %w", err)
	}
	if version > postgresSchemaVersion {
		return fmt.Errorf("PostgreSQL schema version is %d, maximum supported is %d", version, postgresSchemaVersion)
	}
	if version < 1 {
		if _, err := transaction.Exec(ctx, initialSchema); err != nil {
			return fmt.Errorf("apply initial PostgreSQL schema: %w", err)
		}
		if _, err := transaction.Exec(ctx, "INSERT INTO collaboration_schema_migrations(version) VALUES(1)"); err != nil {
			return fmt.Errorf("record initial PostgreSQL schema: %w", err)
		}
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit PostgreSQL migration: %w", err)
	}
	return nil
}
