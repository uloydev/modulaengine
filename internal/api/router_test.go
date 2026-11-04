package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uloydev/modulaengine/internal/api"
	"github.com/uloydev/modulaengine/internal/command"
	"github.com/uloydev/modulaengine/internal/core"
	"github.com/uloydev/modulaengine/internal/query"
)

type mockTxDB struct {
	pingFunc func(ctx context.Context) error
}

func (m *mockTxDB) Begin(ctx context.Context) (pgx.Tx, error) {
	return nil, nil
}

func (m *mockTxDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, nil
}

func (m *mockTxDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return nil
}

func (m *mockTxDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag(""), nil
}

func (m *mockTxDB) Ping(ctx context.Context) error {
	if m.pingFunc != nil {
		return m.pingFunc(ctx)
	}
	return nil
}

func TestRouter_PublicEndpoints(t *testing.T) {
	t.Parallel()

	cfg := api.ServerConfig{
		JWTSecret: []byte("test-key-32bytes-secret-12345678"),
		Version:   "v1.0.0-test",
		Commit:    "abc1234",
		BuildTime: "2026-11-04T12:00:00Z",
	}

	db := &mockTxDB{}
	router := api.NewRouter(cfg, db, nil, nil, nil)

	t.Run("livez endpoint", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/livez", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("readyz endpoint healthy", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("readyz endpoint unhealthy", func(t *testing.T) {
		t.Parallel()
		unhealthyDB := &mockTxDB{
			pingFunc: func(ctx context.Context) error {
				return errors.New("connection refused")
			},
		}
		unhealthyRouter := api.NewRouter(cfg, unhealthyDB, nil, nil, nil)
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()

		unhealthyRouter.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
	})

	t.Run("healthz endpoint", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("version endpoint", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/version", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})
}

func TestRouter_ProtectedCollectionNotFound(t *testing.T) {
	t.Parallel()

	cfg := api.ServerConfig{
		JWTSecret: []byte("test-key-32bytes-secret-12345678"),
	}

	registry := query.NewRegistry()
	queryCompiler := query.NewCompiler(registry)
	cmdCompiler := command.NewCompiler()
	executor := command.NewExecutor()
	hub := api.NewSSEHub()
	db := &mockTxDB{}

	handler := api.NewDataHandler(db, registry, queryCompiler, cmdCompiler, executor, hub)
	router := api.NewRouter(cfg, db, handler, hub, nil)

	secCtx := &core.SecurityContext{
		TenantID: 1,
		UserID:   42,
	}
	token, _ := api.GenerateTestToken(cfg.JWTSecret, secCtx, 15*time.Minute)

	req := httptest.NewRequest(http.MethodGet, "/api/data/nonexistent_col", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
