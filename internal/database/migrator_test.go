package database_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/uloydev/modulaengine/internal/database"
)

type mockMigratorTx struct {
	execFunc   func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	commitFunc func(ctx context.Context) error
}

func (m *mockMigratorTx) Begin(ctx context.Context) (pgx.Tx, error) {
	return m, nil
}

func (m *mockMigratorTx) Commit(ctx context.Context) error {
	if m.commitFunc != nil {
		return m.commitFunc(ctx)
	}
	return nil
}

func (m *mockMigratorTx) Rollback(ctx context.Context) error {
	return nil
}

func (m *mockMigratorTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, nil
}

func (m *mockMigratorTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return nil
}

func (m *mockMigratorTx) LargeObjects() pgx.LargeObjects {
	return pgx.LargeObjects{}
}

func (m *mockMigratorTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return nil, nil
}

func (m *mockMigratorTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, sql, args...)
	}
	return pgconn.NewCommandTag("OK 1"), nil
}

func (m *mockMigratorTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, nil
}

func (m *mockMigratorTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return nil
}

func (m *mockMigratorTx) Conn() *pgx.Conn {
	return nil
}

type mockMigratorDB struct {
	beginFunc func(ctx context.Context) (pgx.Tx, error)
	queryFunc func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (m *mockMigratorDB) Begin(ctx context.Context) (pgx.Tx, error) {
	if m.beginFunc != nil {
		return m.beginFunc(ctx)
	}
	return &mockMigratorTx{}, nil
}

func (m *mockMigratorDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("OK 1"), nil
}

func (m *mockMigratorDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, sql, args...)
	}
	return nil, nil
}

func (m *mockMigratorDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return nil
}

type mockRows struct {
	versions []int
	idx      int
}

func (r *mockRows) Close() {}

func (r *mockRows) Err() error {
	return nil
}

func (r *mockRows) CommandTag() pgconn.CommandTag {
	return pgconn.NewCommandTag("SELECT")
}

func (r *mockRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func (r *mockRows) Next() bool {
	return r.idx < len(r.versions)
}

func (r *mockRows) Scan(dest ...any) error {
	if len(dest) > 0 {
		if v, ok := dest[0].(*int); ok {
			*v = r.versions[r.idx]
			r.idx++
			return nil
		}
	}
	return errors.New("scan error")
}

func (r *mockRows) Values() ([]any, error) {
	return nil, nil
}

func (r *mockRows) RawValues() [][]byte {
	return nil
}

func (r *mockRows) Conn() *pgx.Conn {
	return nil
}

func (r *mockRows) TypeMap() *pgtype.Map {
	return pgtype.NewMap()
}

func TestMigrator_Run_AppliesPending(t *testing.T) {
	t.Parallel()

	executedSQL := make([]string, 0)
	tx := &mockMigratorTx{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			executedSQL = append(executedSQL, sql)
			return pgconn.NewCommandTag("OK 1"), nil
		},
	}

	db := &mockMigratorDB{
		beginFunc: func(ctx context.Context) (pgx.Tx, error) {
			return tx, nil
		},
		queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			// No migrations applied yet
			return &mockRows{versions: []int{}}, nil
		},
	}

	testMigrations := []database.Migration{
		{
			Version:     1,
			Description: "baseline sys tables",
			SQL:         "CREATE TABLE sys_tenants (...);",
		},
	}

	migrator := database.NewMigratorWithMigrations(db, testMigrations)
	err := migrator.Run(context.Background())
	if err != nil {
		t.Fatalf("migrator.Run() unexpected error: %v", err)
	}

	foundDDL := false
	for _, sql := range executedSQL {
		if sql == "CREATE TABLE sys_tenants (...);" {
			foundDDL = true
			break
		}
	}
	if !foundDDL {
		t.Errorf("expected migration DDL to be executed")
	}
}

func TestMigrator_Run_SkipsAlreadyApplied(t *testing.T) {
	t.Parallel()

	executedDDL := false
	tx := &mockMigratorTx{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if sql == "CREATE TABLE sys_tenants (...);" {
				executedDDL = true
			}
			return pgconn.NewCommandTag("OK 1"), nil
		},
	}

	db := &mockMigratorDB{
		beginFunc: func(ctx context.Context) (pgx.Tx, error) {
			return tx, nil
		},
		queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			// Version 1 already applied
			return &mockRows{versions: []int{1}}, nil
		},
	}

	testMigrations := []database.Migration{
		{
			Version:     1,
			Description: "baseline sys tables",
			SQL:         "CREATE TABLE sys_tenants (...);",
		},
	}

	migrator := database.NewMigratorWithMigrations(db, testMigrations)
	err := migrator.Run(context.Background())
	if err != nil {
		t.Fatalf("migrator.Run() unexpected error: %v", err)
	}

	if executedDDL {
		t.Errorf("expected already applied migration to be skipped")
	}
}

func TestPoolManagerWithRetry_ContextCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel context

	cfg := database.Config{
		DataURL:    "postgres://localhost:5432/nonexistent",
		ControlURL: "postgres://localhost:5432/nonexistent",
	}

	_, err := database.NewPoolManagerWithRetry(ctx, cfg, 3, 10*time.Millisecond)
	if err == nil {
		t.Fatalf("expected error from canceled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
}
