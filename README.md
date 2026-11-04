# ModulaEngine

[![Go Version](https://img.shields.io/badge/go-1.22%2B-blue.svg)](https://golang.org)
[![PostgreSQL](https://img.shields.io/badge/postgresql-14%2B-336791.svg)](https://www.postgresql.org)
[![License](https://img.shields.io/badge/license-Apache%202.0-green.svg)](LICENSE)
[![Memory Footprint](https://img.shields.io/badge/idle%20RSS-%3C60MB-brightgreen.svg)]()
[![Binary Size](https://img.shields.io/badge/binary-%3C45MB-blue.svg)]()

ModulaEngine is a lightweight, high-performance, metadata-driven application platform built in pure Go and backed by PostgreSQL. It operates as both a headless Backend-as-a-Service (BaaS) and a fullstack visual application with an embedded Svelte 5 admin console, with an idle footprint strictly under 60MB RSS.

---

## Key Architectural Invariants

- **Single Binary Footprint**: Compiles to a self-contained executable under 45MB. Zero external runtime dependencies (no Node.js/JVM in production).
- **Zero-Trust Multi-Tenancy**: Every compiled query unconditionally injects `AND tenant_id = $tenant_id` at the root AST level. Physical composite foreign keys `FOREIGN KEY (tenant_id, fk_id) REFERENCES usr_target(tenant_id, id)` prevent database-level cross-tenant leakage.
- **Dual-Identifier Paradigm**: Internal `id BIGINT GENERATED ALWAYS AS IDENTITY` for high-throughput clustered B-Tree joins. External `public_id UUID` exposed across APIs, UI, and webhooks. Internal BIGINTs never leak to client JSON payloads.
- **AST Parameterized SQL Compiler**: Zero dynamic string concatenation for SQL. 100% parameter binding (`$1, $2, ...`). Dynamic table/field symbols whitelisted against in-memory metadata registry.
- **Logical CQRS Architecture**: Strict separation of Read Path (`internal/query`) with FLS column projection and Dataloader batching, and Write Path (`internal/command`) with optimistic locking (`version`), atomic CTE audit logging, and SSE dispatch.
- **Embedded Job Queue**: PostgreSQL-native job queue using `FOR UPDATE SKIP LOCKED` with crash reaper daemon. Zero Redis/RabbitMQ operational dependencies.
- **Anti-Overfitting Principle**: Zero domain-specific logic in Go core packages. All domain behavior declared via JSON schema manifests loaded at runtime.

---

## System Architecture

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
│                 PostgreSQL Database (14+)                   │
│  ┌─────────────────────────────┐ ┌───────────────────────┐  │
│  │ System Metadata (sys_*)     │ │ User Data (usr_*)     │  │
│  │ - sys_tenants / sys_users   │ │ - usr_orders          │  │
│  │ - sys_collections / fields  │ │ - usr_customers       │  │
│  │ - sys_permissions / logs    │ │ - ...                 │  │
│  └─────────────────────────────┘ └───────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

---

## Quickstart

### Prerequisites
- **Go**: 1.22 or higher
- **PostgreSQL**: 14 or higher
- **Make**: Standard GNU Make

### 1. Clone Repository
```bash
git clone https://github.com/uloydev/modulaengine.git
cd modulaengine
```

### 2. Configure Environment
```bash
cp .env.example .env
```
Edit `.env` to configure your PostgreSQL connection pools:
- `DATABASE_URL`: Data pool user (`modula_runtime`, DML only)
- `DATABASE_CONTROL_URL`: Control pool user (`modula_control`, DDL & migrations)

### 3. Build & Run
```bash
# Build single binary
make build

# Run quality gate verification
make verify

# Start development server
make run
```

---

## Documentation Index

Comprehensive FAANG-grade specifications are located in the [`docs/`](docs/) directory:

| Document | Description |
|---|---|
| [`docs/architecture/system-design.md`](docs/architecture/system-design.md) | High-level system architecture, CQRS data flow, caching, dual pools, and failure modes |
| [`docs/architecture/competitive-analysis.md`](docs/architecture/competitive-analysis.md) | Deep technical competitor teardown (PocketBase, Supabase, Directus) and strategic moats |
| [`docs/architecture/storage-engine-and-ddl.md`](docs/architecture/storage-engine-and-ddl.md) | Physical table layouts (`usr_*`), composite key modeling, lock timeouts, and tombstoning |
| [`docs/architecture/security-model.md`](docs/architecture/security-model.md) | Zero-trust multi-tenancy, FLS column projection, RLS AST injection, and envelope encryption |
| [`docs/api/rest-api-spec.md`](docs/api/rest-api-spec.md) | Complete RESTful collection API, filtering operators, sorting, pagination, and SSE streaming |
| [`docs/development/coding-guidelines.md`](docs/development/coding-guidelines.md) | FAANG-grade Go standards, memory limits, concurrency rules, and anti-slop comment hygiene |
| [`docs/development/ui-design-guidelines.md`](docs/development/ui-design-guidelines.md) | Svelte 5 runes, virtual data grid architecture, accessibility, and visual anti-slop rules |
| [`docs/development/contributing.md`](docs/development/contributing.md) | Engineering standards, Go conventions, table-driven testing, and pre-commit gates |
| [`AGENTS.md`](AGENTS.md) | Operational guidelines and non-negotiable invariants for AI coding agents |

---

## AI Agent Assisted Environment (`.kilo/`)

This repository is pre-configured with a specialized AI agent team in `.kilo/`:

- **Specialized Subagents**:
  - `architect`: Tech lead & architectural guardian. Enforces invariants and layer boundaries.
  - `database-specialist`: PostgreSQL DDL compiler, composite keys, and lock management.
  - `security-reviewer`: Multi-tenancy boundaries, FLS/RLS, and AST injection auditing.
  - `qa-engineer`: Concurrency testing, race detection (`-race`), and latency benchmarking.
- **Slash Commands**:
  - `/verify`: Pre-commit lint and race detection quality gate (`make verify`).
  - `/db-migrate`: Validates and dry-runs schema DDL migrations.
  - `/review`: Multi-axis architectural and security code review.
  - `/benchmark`: Audits p95 latency targets and heap allocations.

---

## Verification & Quality Gates

Every code change must pass the pre-commit gate before merging:

```bash
make verify
```

Runs:
1. `golangci-lint run ./...` / `go vet ./...` (zero lint warnings).
2. `go test -v -race -short ./...` (zero test failures, zero race conditions).

---

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.
