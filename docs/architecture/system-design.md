# ModulaEngine: System Architecture & Technical Design

**Status**: Production Specification  
**Version**: 1.0.0-MVP  
**Classification**: Engineering Blueprint  

---

## 1. System Overview

ModulaEngine is a lightweight, high-performance, metadata-driven application platform built in pure Go and backed by PostgreSQL. It operates as both a headless BaaS API engine and a fullstack visual app with an embedded Svelte 5 admin console, with an idle footprint strictly under 60MB RSS.

```
┌─────────────────────────────────────────────────────────────┐
│                 Client Layer (Pluggable)                    │
│  ┌─────────────────────────┐   ┌─────────────────────────┐  │
│  │ Embedded Admin SPA      │   │ External Applications   │  │
│  │ (Svelte 5 / Tailwind)   │   │ (Mobile / React / cURL) │  │
│  └────────────┬────────────┘   └────────────┬────────────┘  │
└───────────────┼─────────────────────────────┼───────────────┘
                │ HTTP REST / JSON / SSE
┌───────────────▼─────────────────────────────▼───────────────┐
│                 ModulaEngine Core Runtime                   │
│                                                             │
│  [ Router & Auth Middleware (JWT + Tenant Resolution) ]     │
│                            │                                │
│                            ▼                                │
│  [ In-Memory Metadata Registry (RWMutex Thread-Safe Cache) ]│
│                            │                                │
│                            ▼                                │
│  [ Security AST Injector (FLS projection + RLS filters) ]   │
│                            │                                │
│                            ▼                                │
│  [ Parameterized SQL Compiler ($1, $2, ...) ]               │
│                            │                                │
│             ┌──────────────┴──────────────┐                 │
│             ▼                             ▼                 │
│      [ Query Path ]               [ Command Path ]          │
│    (Read Dataloader)            (Optimistic Lock + CTE)     │
│             │                             │                 │
│             ▼                             ▼                 │
│  [ Data Pool (DML restricted) ] [ Control Pool (DDL only) ] │
│             │                             │                 │
│             ▼                             ▼                 │
│  [ Realtime SSE Event Hub ]  [ PostgreSQL Job Queue Worker ]│
└─────────────┼─────────────────────────────┼─────────────────┘
              │                             │
              ▼                             ▼
┌─────────────────────────────────────────────────────────────┐
│                 PostgreSQL 14+ Instance                     │
│  ┌─────────────────────────────┐ ┌───────────────────────┐  │
│  │ System Metadata (sys_*)     │ │ User Data (usr_*)     │  │
│  │ - sys_tenants / sys_users   │ │ - usr_orders          │  │
│  │ - sys_collections / fields  │ │ - usr_customers       │  │
│  │ - sys_permissions / logs    │ │ - ...                 │  │
│  └─────────────────────────────┘ └───────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

---

## 2. Core Architectural Pillars

### 2.1 Dual-Identifier Strategy & Cache Optimization
To prevent enumeration attacks while maintaining maximum relational join performance:
- **Internal ID (`id BIGINT GENERATED ALWAYS AS IDENTITY`)**: Used exclusively for primary keys, internal database joins, clustered indexes, and B-Tree cache locality.
- **Public ID (`public_id UUID NOT NULL DEFAULT gen_random_uuid()`)**: Exposed to external clients, API payloads, webhooks, and UI URLs.

#### The ID Resolution Pipeline
1. Incoming mutation requests (`POST`/`PATCH`) reference related records via UUID (`customer_id: "018e32c8-..."`).
2. An in-memory LRU cache (`tenant_id:collection:public_id -> bigint_id`) translates UUIDs to internal `BIGINT`s in sub-microsecond time.
3. Outgoing responses strip internal `id` entirely and map `public_id` to `"id"`.

### 2.2 Physical Tenant Isolation Invariant
Application-level filtering alone is insufficient for zero-trust multi-tenancy. ModulaEngine enforces composite foreign keys on all generated tables:

```sql
-- Enforce tenant-scoped uniqueness
ALTER TABLE usr_customers ADD CONSTRAINT uq_customers_tenant_id UNIQUE (tenant_id, id);

-- Enforce physical cross-tenant isolation on joins
ALTER TABLE usr_orders 
  ADD CONSTRAINT fk_orders_customer 
  FOREIGN KEY (tenant_id, customer_id) 
  REFERENCES usr_customers(tenant_id, id) 
  ON DELETE RESTRICT;
```
A record belonging to Tenant A can never reference a record belonging to Tenant B at the database engine level.

### 2.3 Logical CQRS Pattern (Single DB, Dual Execution Path)
Separates read pipelines and write pipelines while retaining immediate ACID consistency:
- **Query Path (`internal/query`)**:
  - Validates requested field projection against FLS role mask.
  - Builds AST with injected tenant filter and RLS expression.
  - Compiles parameterized SQL with bounded array sizes.
  - Uses per-request Dataloader instances for relation expansion without N+1 queries.
- **Command Path (`internal/command`)**:
  - Enforces schema validation and type coercion.
  - Evaluates optimistic locking (`WHERE public_id = $1 AND version = $2`).
  - Executes atomic CTE to update record and emit audit diff record simultaneously.
  - Dispatches mutation events to the SSE event hub and background job queue.

### 2.4 Query Path Accelerators (Dataloader & CompiledQueryCache)

#### Relation Dataloader (`internal/query/dataloader.go`)
Resolving nested relations (`?expand=customer,account`) without batching causes severe N+1 latency degradation. ModulaEngine employs a per-request `RelationResolver`:
1. Collects and deduplicates foreign key identifiers across the current pagination batch (up to 100 records).
2. Executes a single batched query per related collection:
   ```sql
   SELECT <allowed_fields> 
   FROM usr_customers 
   WHERE id = ANY($1) AND tenant_id = $2 AND deleted_at IS NULL;
   ```
3. Hydrates relation objects into parent records in Go memory before serializing response JSON.
4. Lifecyle is bound to the request context to guarantee thread isolation across concurrent clients.

#### Compiled Query LRU Cache (`internal/query/cache.go`)
To eliminate repetitive AST parsing, symbol validation, and predicate rewriting on high-frequency read endpoints, compiled query metadata is stored in an in-memory `CompiledQueryCache`:
- **Cache Key**: Deterministic SHA-256 hash computed over `tenant_id`, collection name, raw URL query string, and the user's role identifier.
- **Data Structure**: Mutex-synchronized doubly linked list with hash table indexing (default capacity: 10,000 entries).
- **Eviction**: Least-Recently-Used (LRU) tail eviction when capacity is reached. Cached entries return deep copies of parameterized SQL and projection descriptors to prevent shared mutation across goroutines.

---

## 3. High-Throughput Job Queue (`sys_jobs`)

To avoid external broker dependencies (Redis, RabbitMQ), ModulaEngine embeds a PostgreSQL-backed job queue leveraging atomic row locking:

```sql
UPDATE sys_jobs 
SET status = 'RUNNING', 
    locked_by = $worker_id, 
    locked_at = now()
WHERE id = (
    SELECT id FROM sys_jobs 
    WHERE status = 'PENDING' AND run_at <= now() 
    ORDER BY run_at ASC 
    FOR UPDATE SKIP LOCKED 
    LIMIT 1
) RETURNING *;
```

### Worker Reliability & Crash Recovery
- **Lock Timeout Reaper**: A background loop executes every 60 seconds to release stranded jobs (`status = 'RUNNING' AND locked_at < now() - INTERVAL '5 minutes'`).
- **Retry Backoff**: Exponential backoff (`run_at = now() + (2 ^ attempts * 10 seconds)`). Max attempts default: 3.
- **Poison Pill Handling**: Jobs exceeding `max_attempts` transition to `FAILED` with `last_error` populated.

---

## 4. Resource Allocation & Latency Budgets

| Metric | Target Budget | Enforcement Mechanism |
|---|---|---|
| Idle RSS Memory | < 60 MB | Go runtime GC tuning (`GOMEMLIMIT=128MiB`), lazy pool init |
| Binary Size | < 45 MB | Compressed Svelte 5 static embed (`//go:embed`), `-s -w` ldflags |
| Read Latency (p95) | < 15 ms | Pre-compiled prepared statements, B-tree index coverage |
| Write Latency (p95) | < 35 ms | Single-roundtrip CTE execution, async audit dispatch |
| DDL Lock Timeout | 2.0 s | `SET LOCAL lock_timeout = '2s'` on control pool |
| Request Timeout | 5.0 s | Context deadline cancellation on all HTTP handlers |

---

## 5. Configuration Engine & Credential Hardening (`internal/config`)

ModulaEngine enforces the 12-factor configuration hierarchy:
$$\text{Defaults} \longrightarrow \text{.env File} \longrightarrow \text{OS Environment Variables} \longrightarrow \text{CLI Runtime Overrides}$$

### 5.1 Process Table Credential Protection (CWE-214)
Accepting database passwords or encryption secrets via CLI arguments exposes plaintext credentials to unprivileged local users via `ps aux`, `/proc/<PID>/cmdline`, and container inspection APIs.
- CLI flags `--db`, `--db-control`, and `--jwt-secret` are eliminated.
- Database connection strings (`DATABASE_URL`, `DATABASE_CONTROL_URL`, `DATABASE_READ_URL`) and cryptographic keys (`JWT_SECRET`) must be provided via `.env` or OS environment variables.
- CLI flags are strictly restricted to non-sensitive runtime controls: `--env-file`, `--port`, `--headless`, `--log-level`, `--log-file`, `--log-console`, `--version`.

### 5.2 Zero-Dependency `.env` Loader
`internal/config/env.go` implements an in-engine parser:
- Parses `KEY=VALUE` pairs with support for `#` comments, whitespace trimming, and quoted strings.
- Respects existing OS environment variables: variables already set in the operating system environment are never overwritten by `.env` entries.

### 5.3 Production Environment Validation
When `ENV=production`:
- `JWT_SECRET` must be at least 32 bytes in length.
- Default development secrets and placeholder strings containing `change-in-production` cause immediate configuration failure (`ErrWeakSecret`).

---

## 6. Application Lifecycle Container (`internal/app`)

The application lifecycle is decoupled from CLI entrypoints via the `App` container (`internal/app/app.go`), coordinating startup sequencing, runtime supervision, and graceful teardown.

```
       ┌────────────────────────┐
       │     app.New(cfg)       │
       └───────────┬────────────┘
                   │
                   ▼
       ┌────────────────────────┐
       │   Pool Retry Connect   │ (Exponential backoff)
       └───────────┬────────────┘
                   │
                   ▼
       ┌────────────────────────┐
       │  sys_* Schema Migrator │ (Control Pool DDL)
       └───────────┬────────────┘
                   │
                   ▼
       ┌────────────────────────┐
       │      app.Start()       │ (Queue Workers + TCP Listener)
       └───────────┬────────────┘
                   │ (SIGINT / SIGTERM)
                   ▼
       ┌────────────────────────┐
       │      app.Stop(ctx)     │ (HTTP drain -> Workers -> DB pools)
       └────────────────────────┘
```

### 6.1 Lifecycle Methods
- `app.New(ctx, cfg, version, commit, buildTime)`: Initializes database pools with cold-boot retry, runs catalog migrations, constructs AST compilers, builds the Chi HTTP router, and wires background queue workers.
- `app.Start()`: Starts queue worker pool and lock reaper goroutines, binds the TCP listener, and begins serving HTTP traffic.
- `app.Stop(ctx)`: Coordinates ordered shutdown:
  1. Shuts down the HTTP server with bounded context deadline (5s).
  2. Cancels worker context to stop job dequeue loops.
  3. Stops the crash reaper daemon.
  4. Closes PostgreSQL connection pools (`DataPool`, `ControlPool`).

---

## 7. System Catalog Bootstrap Migrator (`internal/database/migrator.go`)

To eliminate manual bootstrapping of system tables, ModulaEngine executes transactional baseline migrations on boot using the Control Pool.

### 7.1 Migration Execution & Versioning
- `sys_schema_version`: Tracks applied migrations with `version INT PRIMARY KEY`, `description TEXT`, and `applied_at TIMESTAMPTZ`.
- Migrations execute sequentially within atomic transactions:
  ```sql
  BEGIN;
  SET LOCAL lock_timeout = '2s';
  SET LOCAL statement_timeout = '5s';
  -- Execute baseline DDL statements
  INSERT INTO sys_schema_version (version, description) VALUES ($1, $2);
  COMMIT;
  ```
- **Baseline Tables**: `sys_tenants`, `sys_users`, `sys_roles`, `sys_permissions`, `sys_collections`, `sys_fields`, `sys_jobs`, `sys_audit_logs`.

### 7.2 Cold-Boot Connection Retry
In container environments where PostgreSQL boots concurrently with the application, immediate connection failure causes crash loops. `NewPoolManagerWithRetry` implements exponential backoff:
- Default: 5 attempts with 500ms initial backoff and jitter (500ms, 1s, 2s, 4s, 8s).
- Re-verifies connectivity via `Ping()` before proceeding to schema migration.

---

## 8. Health Probes & Observability Architecture

ModulaEngine exposes distinct probe endpoints configured for container orchestrators (Kubernetes, Nomad, Docker Compose):

```
                       ┌──────────────┐
                       │ Orchestrator │
                       └──────┬───────┘
                              │
          ┌───────────────────┼───────────────────┐
          │ GET /livez        │ GET /readyz       │ GET /version
          ▼                   ▼                   ▼
┌───────────────────┐ ┌───────────────────┐ ┌───────────────────┐
│ Event Loop Alive  │ │ Database Ping OK  │ │ Build Metadata    │
│ HTTP 200 {"alive"}│ │ HTTP 200 {"ready"}│ │ Commit / Version  │
└───────────────────┘ └───────────────────┘ └───────────────────┘
```

### 8.1 Probe Contracts
- `GET /livez`: Liveness probe. Returns HTTP 200 `{"status":"alive"}` immediately without database I/O. Used by orchestrators to detect process deadlock or event loop failure.
- `GET /readyz`: Readiness probe. Executes `db.Ping(ctx)` on the database pool. Returns HTTP 200 `{"status":"ready"}` when connected, or HTTP 503 `Service Unavailable` with RFC 7807 problem details if database connectivity is severed. Prevents orchestrators from routing traffic to unready instances.
- `GET /healthz`: Backward-compatible readiness alias routing to the `/readyz` handler.
- `GET /version`: Returns runtime version, git commit hash, and build timestamp.
