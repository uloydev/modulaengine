# ModulaEngine: Complete Architecture & Code Quality Hardening Plan

**Plan File**: `.kilo/plans/1793724149255-complete-architecture-suite.md`  
**Status**: Implementation Ready  
**Scope**: Complete Architecture Suite (Configuration, App Lifecycle, Schema Migrator, Probes, Dataloader, Caching, Integration Tests)  
**Target Stack**: Go 1.22+, PostgreSQL 14+, Zerolog, Chi router, Svelte 5  

---

## 1. Problem Statement & Architecture Context

ModulaEngine's core functional components (dynamic DDL, AST compiler, optimistic concurrency, SKIP LOCKED queue, REST API, embedded Svelte 5 SPA) are implemented and functional. However, several enterprise operational deficiencies and code smells remain:

1. **Process Table Credential Leakage (CWE-214)**:
   - Command-line flags currently accept `--db`, `--db-control`, and `--jwt-secret`.
   - Any local OS user or monitoring daemon executing `ps aux` or reading `/proc/<PID>/cmdline` can inspect database passwords and cryptographic keys in plaintext.
   - Violates 12-Factor App methodology and CIS/OWASP security standards.
2. **Missing System Catalog Bootstrap (`sys_*`)**:
   - `sys_*` tables (tenants, users, roles, permissions, collections, fields, jobs, audit_logs) are expected to exist, but there is no automated schema migration runner to bootstrap them on first run.
3. **Monolithic Startup in `cmd/modulaengine/main.go`**:
   - `main.go` contains 175 lines of imperative component wiring, mixing flag parsing, logger setup, pool connection, worker dispatch, router creation, and teardown.
4. **Coarse Health Probes**:
   - Only a single `/healthz` endpoint exists. Orchestrators (Kubernetes, Nomad, Docker Compose) require separate `/livez` (process alive) and `/readyz` (pools connected and ready for traffic) probes.
5. **No Cold-Boot Database Retry**:
   - If PostgreSQL starts 2 seconds slower than the ModulaEngine container, `NewPoolManager` immediately aborts with `os.Exit(1)`.
6. **Sequential Relation Expansion (N+1 Risk)**:
   - Relation expansion (`?expand=...`) lacks a batched Dataloader pattern, executing individual queries per record.

---

## 2. Target Design & Technical Specifications

### 2.1 Configuration Engine (`internal/config`)

Enforces the 12-factor configuration hierarchy:
$$\text{Defaults} \longrightarrow \text{.env File} \longrightarrow \text{OS Environment Variables} \longrightarrow \text{CLI Runtime Overrides}$$

#### Rules:
- **No secret CLI flags**: Completely remove `--db`, `--db-control`, and `--jwt-secret`.
- **Allowed CLI flags** (strictly non-sensitive runtime controls):
  - `--env-file`: Custom path to `.env` file (default: `.env`).
  - `--port`: HTTP listen port override.
  - `--headless`: Drop UI static embed.
  - `--log-level`: Log level override.
  - `--log-console`: Console formatting toggle.
  - `--version`: Print build metadata and exit.
- **Environment Variable Mapping**:
  - `PORT`: HTTP port (default: 8080).
  - `ENV`: Environment mode (`development`, `production`).
  - `DATABASE_URL`: Runtime DML pool connection string (required).
  - `DATABASE_CONTROL_URL`: Migration/DDL pool connection string (required).
  - `DATABASE_READ_URL`: Optional read-replica connection string.
  - `JWT_SECRET`: Signing secret (minimum 32 bytes in production).
  - `LOG_LEVEL`: `debug`, `info`, `warn`, `error` (default: `info`).
  - `LOG_FILE`: Lumberjack destination path (optional).
  - `LOG_CONSOLE`: Pretty console output boolean (default: `false`).
  - `HEADLESS`: Headless mode toggle (default: `false`).
- **Zero-Dependency `.env` Parser**:
  - Implemented in `internal/config/env.go`.
  - Supports `KEY=VALUE`, `#` comments, trimmed whitespace, and unquoted/quoted string values.
  - Never overwrites already-set OS environment variables (OS env takes precedence over `.env`).

```go
type Config struct {
    Server   ServerConfig
    Database DatabaseConfig
    Security SecurityConfig
    Log      LogConfig
}
```

---

### 2.2 Application Container & Lifecycle (`internal/app`)

Decouples wiring and teardown from `cmd/modulaengine/main.go`:

```go
type App struct {
    cfg         *config.Config
    logger      zerolog.Logger
    poolMgr     *database.PoolManager
    registry    *query.Registry
    workerPool  *queue.WorkerPool
    reaper      *queue.Reaper
    httpServer  *http.Server
}

func New(ctx context.Context, cfg *config.Config) (*App, error)
func (a *App) Start(ctx context.Context) error
func (a *App) Stop(ctx context.Context) error
```

`cmd/modulaengine/main.go` shrinks to <40 lines: parse runtime flags, load config, instantiate `app.New()`, block on signal channel, call `app.Stop()`.

---

### 2.3 System Catalog Bootstrap Migrator (`internal/database/migrator.go`)

Runs automatically on boot using the **Control Pool** inside an explicit transaction:

#### Invariants:
1. `sys_schema_version` tracks executed migration scripts.
2. Bootstraps base catalog tables:
   - `sys_tenants`: `id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY`, `public_id UUID`, `name`, `slug`, `is_active`, `created_at`, `updated_at`.
   - `sys_users`: `id`, `public_id`, `tenant_id REFERENCES sys_tenants(id)`, `email`, `password_hash`, `role_id`, `is_active`, timestamps.
   - `sys_roles`: `id`, `tenant_id`, `name`, `description`.
   - `sys_permissions`: `id`, `tenant_id`, `role_id`, `collection_id`, `actions TEXT[]`, `allowed_fields TEXT[]`, `rls_filter TEXT`.
   - `sys_collections`: `id`, `tenant_id`, `name`, `table_name`, `label`, `is_system`, timestamps.
   - `sys_fields`: `id`, `collection_id`, `name`, `type`, `required`, `unique`, `searchable`, `is_encrypted`, `default_value`, `options TEXT[]`, `precision`, `scale`, timestamps.
   - `sys_jobs`: `id`, `tenant_id`, `workflow_id`, `task_type`, `payload JSONB`, `status`, `run_at`, `attempts`, `max_attempts`, `last_error`, `locked_by`, `locked_at`, `created_at`.
   - `sys_audit_logs`: `id`, `tenant_id`, `collection_id`, `record_id`, `action`, `user_id`, `diff JSONB`, `created_at`.
3. Migrations are idempotent (`CREATE TABLE IF NOT EXISTS` + version tracking).

---

### 2.4 Cold-Boot Connection Retry Backoff

In `internal/database/pool.go`:
- `NewPoolManagerWithRetry(ctx context.Context, cfg Config, maxRetries int, backoff time.Duration)`
- Retries pool ping with exponential backoff and jitter (e.g., 5 attempts: 500ms, 1s, 2s, 4s, 8s).
- Ensures seamless startup in containerized environments (Docker Compose, Kubernetes) where database service discovery takes a few seconds.

---

### 2.5 Probe Endpoints & Centralized Error Normalization (`internal/api`)

#### Probes:
- `GET /livez`: Instant HTTP 200 `{"status": "alive"}`. Validates HTTP event loop responsiveness.
- `GET /readyz`: Pings both `DataPool` and `ControlPool`. If either pool fails, returns HTTP 503 `{"status": "not_ready", "error": "..."}`.
- `GET /healthz`: Backward-compatible alias routing to `/readyz`.

#### Centralized Error Normalization:
Create `internal/api/error_mapper.go`:
```go
func MapDomainError(err error) (statusCode int, title string) {
    switch {
    case errors.Is(err, core.ErrNotFound):
        return http.StatusNotFound, "Record Not Found"
    case errors.Is(err, core.ErrVersionConflict):
        return http.StatusConflict, "Optimistic Concurrency Conflict"
    case errors.Is(err, query.ErrFieldNotPermitted):
        return http.StatusForbidden, "Field Access Forbidden"
    case errors.Is(err, core.ErrTenantMismatch):
        return http.StatusUnauthorized, "Tenant Authorization Failure"
    case errors.Is(err, core.ErrValidation), errors.Is(err, query.ErrUnknownField), errors.Is(err, core.ErrInvalidIdentifier):
        return http.StatusBadRequest, "Validation Error"
    default:
        return http.StatusInternalServerError, "Internal Server Error"
    }
}
```
Replaces repetitive `if errors.Is(...)` checks in `handlers.go`.

---

### 2.6 Relation Dataloader (`internal/query/dataloader.go`)

Prevents N+1 query patterns when clients request relation expansion (`?expand=customer,account`):
1. Collects all foreign key IDs across the page batch (up to 100 records).
2. Issues a single batched query per related collection:
   ```sql
   SELECT <allowed_fields> FROM usr_customers 
   WHERE id = ANY($1) AND tenant_id = $2 AND deleted_at IS NULL;
   ```
3. Hydrates relation objects in Go memory before writing response JSON.
4. Scoped strictly per-request in `context.Context` (never shared across concurrent requests).

---

### 2.7 Compiled Query LRU Cache (`internal/query/cache.go`)

- Caches compiled SQL string and column descriptors using cache key `SHA256(tenant_id + collection + raw_query_string + role_name)`.
- Eliminates repeated AST parsing and tokenization on high-frequency read endpoints.
- Thread-safe with bounded size (max 10,000 entries) and eviction policy.

---

### 2.8 Live PostgreSQL Integration Test Suite (`tests/integration/postgres_test.go`)

- Validates end-to-end functionality against real PostgreSQL instances:
  - Catalog bootstrap migrator execution.
  - Multi-tenant foreign key restriction (`FOREIGN KEY (tenant_id, fk_id)`).
  - SKIP LOCKED concurrent job claiming across multiple simulated workers.
  - True CTE optimistic locking and audit diffing.
- Guarded by `//go:build integration` tag to keep standard unit tests fast (`make test-race`).

---

## 3. Ordered Implementation Sequence & Subagent Delegation Matrix

| Phase | Description | Assigned Subagent Profile | Verification Target |
|---|---|---|---|
| **Phase 1** | **Configuration & Security Remediation** (`internal/config`) | `security-reviewer` | Zero process table leaks (`ps aux`), `.env` hierarchy |
| **Phase 2** | **App Container & Lifecycle** (`internal/app`) | `architect` | Decoupled `main.go`, graceful `Start`/`Stop` |
| **Phase 3** | **Catalog Migrator & Cold-Boot Resilience** (`internal/database`) | `database-specialist` | Automatic `sys_*` DDL bootstrap, connection retry loop |
| **Phase 4** | **Probes & Error Normalization** (`internal/api`) | `qa-engineer` | `/livez`, `/readyz`, centralized `error_mapper.go` |
| **Phase 5** | **Relation Dataloader & Query Cache** (`internal/query`) | `architect` | Batched `WHERE id = ANY($1)`, compiled query cache |
| **Phase 6** | **Live PostgreSQL Integration Harness** (`tests/integration`) | `qa-engineer` | Multi-tenant isolation, real DB testing |
| **Phase 7** | **Documentation Alignment & Sync** (`docs/`, `README.md`) | `architect` | Sync docs with code changes |
| **Phase 8** | **Multi-Axis Review & QA Gate Verification** | `qa-engineer` + `architect` | `make verify` (zero lint, zero race), commit & push |

---

### Phase 1: Configuration & Security Remediation (Subagent: `security-reviewer`)
1. Implement `internal/config/env.go`: Zero-dependency `.env` file parser with comment/quote handling.
2. Implement `internal/config/config.go`: Typed struct hierarchy, environment variable binding, default values, and production secret strength validation (reject default JWT secret when `ENV=production`).
3. Add unit tests in `internal/config/config_test.go`.
4. Refactor `cmd/modulaengine/main.go` to remove `--db`, `--db-control`, and `--jwt-secret` flags. Keep only `--env-file`, `--port`, `--headless`, `--log-level`, `--log-console`, `--version`.

### Phase 2: Application Lifecycle & Decoupling (Subagent: `architect`)
1. Implement `internal/app/app.go`: Encapsulate pool setup, background workers, reaper, routing, and graceful teardown.
2. Wire `cmd/modulaengine/main.go` to call `app.New(ctx, cfg)`.

### Phase 3: Database Bootstrap Migrator & Cold-Boot Resilience (Subagent: `database-specialist`)
1. Implement `internal/database/migrations.go`: Embedded SQL DDL defining the baseline `sys_*` catalog schema.
2. Implement `internal/database/migrator.go`: Transactional migration runner verifying and executing unapplied schema migrations using the Control Pool.
3. Update `internal/database/pool.go` with exponential backoff retry loop on pool initialization.
4. Add unit and migration tests in `internal/database/migrator_test.go`.

### Phase 4: Probes & Error Normalization (Subagent: `qa-engineer`)
1. Implement `internal/api/error_mapper.go`: Centralized domain-to-HTTP mapping.
2. Update `internal/api/handlers.go` to use `error_mapper.go`.
3. Implement `/livez` and `/readyz` endpoints in `internal/api/router.go`.
4. Update `internal/api/router_test.go` to verify both probes.

### Phase 5: Relation Expansion Dataloader & Query Cache (Subagent: `architect`)
1. Implement `internal/query/dataloader.go`: Per-request batch loader executing `WHERE id = ANY($1)`.
2. Implement `internal/query/cache.go`: Thread-safe LRU cache for compiled queries.
3. Update `internal/api/handlers.go` to invoke Dataloader when `?expand=...` is present.
4. Add unit tests in `internal/query/dataloader_test.go`.

### Phase 6: Live Integration Test Harness (Subagent: `qa-engineer`)
1. Create `tests/integration/postgres_test.go` guarded by `//go:build integration`.
2. Add Makefile target `test-integration`.

### Phase 7: Documentation Alignment & Sync (Subagent: `architect`)
1. Update `README.md` reflecting new 12-factor configuration (removal of `--db` / `--jwt-secret` CLI flags).
2. Update `docs/architecture/system-design.md` with `/livez` & `/readyz` probe contracts and `Dataloader` batching.
3. Update `docs/api/rest-api-spec.md` with new probe endpoints and relation expansion syntax.

### Phase 8: Multi-Axis Review & QA Gate Verification (Subagent: `qa-engineer` + `architect`)
1. Execute `make verify` (`golangci-lint run ./...` + `go test -v -race -short ./...`).
2. Run benchmark check (`make bench`).
3. Verify binary footprint (`make build && ls -lh bin/modulaengine` < 45MB).
4. Stage, commit, and push changes to `origin/main`.

---

## 4. Invariant & Risk Verification Matrix

| Area | Invariant Target | Verification Method |
|---|---|---|
| **Process Table Security** | Zero credentials in `ps aux` | Verify `cmdline` contains only `--port` / `--env-file`, never passwords |
| **Binary Size** | Total binary < 45 MB | `ls -lh bin/modulaengine` must remain < 20 MB |
| **Idle Memory (RSS)** | Idle memory < 60 MB | `ps -o rss` on running instance |
| **Zero-Trust Multi-Tenancy** | Composite keys `(tenant_id, id)` | Integration tests verifying cross-tenant insert failure |
| **Zero Dynamic SQL** | 100% parameter binding (`$1..$N`) | Static inspection and AST compiler table tests |
| **Concurrency Safety** | Zero data races | `go test -v -race ./...` |
| **Code Comment Hygiene** | Zero AI slop / narration | Anti-slop review gate (`docs/development/coding-guidelines.md`) |

---

## 5. Definition of Done

1. All tasks in Phases 1–6 implemented and committed to git.
2. `make verify` passes cleanly (0 lint warnings, 100% unit tests pass with `-race`).
3. Binary size confirmed under 45MB.
4. Updated documentation (`README.md`, `docs/architecture/system-design.md`) reflecting new configuration hierarchy and probe endpoints.
