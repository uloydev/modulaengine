# Engineering Standards & Contribution Guide

**Status**: Production Guidelines  
**Standard**: FAANG-Grade Engineering Discipline  

---

## 1. Go Architecture & Coding Conventions

1. **Package Boundaries**:
   - `cmd/modulaengine`: Application entrypoint, CLI flag parsing, graceful shutdown.
   - `internal/core`: Domain primitives (Tenant, Collection, Field, Record, Schema). Zero framework dependencies.
   - `internal/database`: PostgreSQL connection pool management (`pgxpool`), DDL compiler, migration runner.
   - `internal/query`: AST query parser, parameterized SQL compiler, FLS projection, Dataloader.
   - `internal/command`: Atomic mutation pipeline, optimistic lock validation, CTE audit logger.
   - `internal/queue`: SKIP LOCKED job worker pool and lock-reaper daemon.
   - `internal/api`: HTTP router (`chi`), auth middlewares, SSE hub, REST handlers.
   - `web/`: Embedded Svelte 5 SPA.

2. **Error Handling Invariants**:
   - Wrap errors with context using `%w`: `fmt.Errorf("execute ddl for collection %s: %w", name, err)`.
   - Never discard errors. Handled errors must be logged once with structured attributes (`log/slog`).
   - Use typed sentinel errors for domain failures (`ErrNotFound`, `ErrVersionConflict`, `ErrTenantMismatch`).

3. **Concurrency & Memory Hygiene**:
   - Every goroutine must have an explicit cancellation lifecycle via `context.Context`.
   - Protect shared in-memory caches using `sync.RWMutex` with minimal critical sections.
   - Zero goroutine leaks: All tests must run with `-race` enabled.

---

## 2. PostgreSQL Testing & Validation Standards

1. **Integration Tests**:
   - Use test databases with real PostgreSQL instances. Do not use SQLite mocks for PostgreSQL-specific features (`SKIP LOCKED`, CTEs, JSONB).
   - Each integration test run uses an isolated test tenant.

2. **Table-Driven Test Pattern**:
   ```go
   func TestASTCompiler_Compile(t *testing.T) {
       t.Parallel()
       tests := []struct {
           name        string
           input       FilterAST
           expectedSQL string
           expectedArgs []any
           wantErr     bool
       }{
           // Table cases...
       }
       for _, tt := range tests {
           tt := tt
           t.Run(tt.name, func(t *testing.T) {
               t.Parallel()
               // Assertions...
           })
       }
   }
   ```

---

## 3. Pre-Commit Quality Gate

Before pushing any changes or submitting a PR, the following suite must pass cleanly:

```bash
make verify
```
This executes:
1. `go vet ./...` / `golangci-lint run ./...` (zero lint warnings).
2. `go test -v -race -short ./...` (zero test failures, zero race conditions).
