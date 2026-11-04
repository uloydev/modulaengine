package database

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrMigrationFailed = errors.New("database migration failed")
)

type DB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Migrator struct {
	db         DB
	migrations []Migration
}

func NewMigrator(db DB) *Migrator {
	return &Migrator{
		db:         db,
		migrations: BaselineMigrations,
	}
}

func NewMigratorWithMigrations(db DB, migrations []Migration) *Migrator {
	return &Migrator{
		db:         db,
		migrations: migrations,
	}
}

const createSchemaVersionTableSQL = `
CREATE TABLE IF NOT EXISTS sys_schema_version (
    version INT PRIMARY KEY,
    description TEXT,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

func (mig *Migrator) Run(ctx context.Context) error {
	if err := mig.bootstrapSchemaVersion(ctx); err != nil {
		return fmt.Errorf("bootstrap schema version table: %w", err)
	}

	applied, err := mig.getAppliedVersions(ctx)
	if err != nil {
		return fmt.Errorf("get applied versions: %w", err)
	}

	sortedMigrations := make([]Migration, len(mig.migrations))
	copy(sortedMigrations, mig.migrations)
	sort.Slice(sortedMigrations, func(i, j int) bool {
		return sortedMigrations[i].Version < sortedMigrations[j].Version
	})

	for _, m := range sortedMigrations {
		if applied[m.Version] {
			continue
		}

		if err := mig.applyMigration(ctx, m); err != nil {
			return fmt.Errorf("%w: version %d (%s): %w", ErrMigrationFailed, m.Version, m.Description, err)
		}
		applied[m.Version] = true
	}

	return nil
}

func (mig *Migrator) Pending(ctx context.Context) ([]Migration, error) {
	if err := mig.bootstrapSchemaVersion(ctx); err != nil {
		return nil, fmt.Errorf("bootstrap schema version table: %w", err)
	}

	applied, err := mig.getAppliedVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("get applied versions: %w", err)
	}

	sortedMigrations := make([]Migration, len(mig.migrations))
	copy(sortedMigrations, mig.migrations)
	sort.Slice(sortedMigrations, func(i, j int) bool {
		return sortedMigrations[i].Version < sortedMigrations[j].Version
	})

	var pending []Migration
	for _, m := range sortedMigrations {
		if !applied[m.Version] {
			pending = append(pending, m)
		}
	}

	return pending, nil
}

func (mig *Migrator) bootstrapSchemaVersion(ctx context.Context) error {
	tx, err := mig.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin bootstrap transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if _, err = tx.Exec(ctx, "SET LOCAL lock_timeout = '2s';"); err != nil {
		return fmt.Errorf("set lock_timeout: %w", err)
	}
	if _, err = tx.Exec(ctx, "SET LOCAL statement_timeout = '5s';"); err != nil {
		return fmt.Errorf("set statement_timeout: %w", err)
	}

	if _, err = tx.Exec(ctx, createSchemaVersionTableSQL); err != nil {
		return fmt.Errorf("create sys_schema_version table: %w", err)
	}

	return tx.Commit(ctx)
}

func (mig *Migrator) getAppliedVersions(ctx context.Context) (map[int]bool, error) {
	rows, err := mig.db.Query(ctx, "SELECT version FROM sys_schema_version ORDER BY version ASC;")
	if err != nil {
		return nil, fmt.Errorf("query sys_schema_version: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan migration version: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migration versions: %w", err)
	}

	return applied, nil
}

func (mig *Migrator) applyMigration(ctx context.Context, m Migration) error {
	backoffs := []time.Duration{
		100 * time.Millisecond,
		400 * time.Millisecond,
		1200 * time.Millisecond,
	}

	var lastErr error
	for attempt := 0; attempt <= len(backoffs); attempt++ {
		err := mig.executeTx(ctx, m)
		if err == nil {
			return nil
		}
		lastErr = err

		if !isLockTimeout(err) || attempt == len(backoffs) {
			break
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("migration %d canceled during lock retry: %w", m.Version, ctx.Err())
		case <-time.After(backoffs[attempt]):
		}
	}

	return lastErr
}

func (mig *Migrator) executeTx(ctx context.Context, m Migration) (err error) {
	tx, err := mig.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if _, err = tx.Exec(ctx, "SET LOCAL lock_timeout = '2s';"); err != nil {
		return fmt.Errorf("set lock_timeout: %w", err)
	}
	if _, err = tx.Exec(ctx, "SET LOCAL statement_timeout = '5s';"); err != nil {
		return fmt.Errorf("set statement_timeout: %w", err)
	}

	if _, err = tx.Exec(ctx, m.SQL); err != nil {
		return fmt.Errorf("execute migration ddl: %w", err)
	}

	recordSQL := `INSERT INTO sys_schema_version (version, description, applied_at) VALUES ($1, $2, now());`
	if _, err = tx.Exec(ctx, recordSQL, m.Version, m.Description); err != nil {
		return fmt.Errorf("record migration version: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func isLockTimeout(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "55P03"
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "55p03") || strings.Contains(msg, "lock_not_available") || strings.Contains(msg, "lock timeout")
}
