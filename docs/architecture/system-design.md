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
