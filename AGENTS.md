# AGENTS.md: ModulaEngine AI Agent Operational Guidelines

Welcome to the ModulaEngine codebase. This document establishes non-negotiable architectural invariants, code standards, security rules, and engineering workflows for all AI coding agents working in this repository.

---

## 1. Prime Directive & Core Invariants

1. **Single Binary & Strict Memory Footprint**:
   - The entire engine compiles into a single executable (< 45MB).
   - Idle memory footprint must remain under **60MB RSS**.
   - No heavyweight external runtimes (no Node.js or JVM dependencies in production). Frontend is compiled Svelte 5 embedded via Go's `//go:embed`.

2. **Zero-Trust Multi-Tenancy**:
   - Every compiled query **must unconditionally inject** `AND tenant_id = $tenant_id` at the root AST level.
   - All physical foreign keys on dynamic tables (`usr_*`) must be composite: `FOREIGN KEY (tenant_id, foreign_key_id) REFERENCES usr_target(tenant_id, id)`.
   - Never allow tenant boundaries to rely solely on application filter logic.

3. **Dual-Identifier Paradigm**:
   - Internal keys: `id BIGINT GENERATED ALWAYS AS IDENTITY`. Used for internal joins and B-Tree indexing.
   - External keys: `public_id UUID NOT NULL DEFAULT gen_random_uuid()`. Exposed to APIs, UI, webhooks, and client payloads.
   - Never expose internal `BIGINT id` in external JSON responses. Map `public_id` to `"id"`.

4. **Zero Dynamic SQL String Concatenation**:
   - Every SQL statement must be compiled through the AST Parameterized Compiler.
   - All user inputs must use positional parameter binding (`$1, $2, ...`).
   - Identifiers (table names, column names) must be validated against the in-memory metadata registry whitelist before SQL generation.

5. **Anti-Overfitting Principle**:
   - Zero domain-specific logic in Go packages (no hardcoded references to `leads`, `deals`, `orders`, `tickets`).
   - The Go engine only understands structural primitives: `collections`, `fields`, `relations`, `permissions`, and `events`.
   - All business logic and domain schemas are declared via JSON manifests loaded at runtime.

---

## 2. Directory Structure & Layer Boundaries

```
.
├── cmd/
│   └── modulaengine/         # Application entrypoint & CLI command wiring
├── internal/
│   ├── core/                 # Pure domain primitives (Tenant, Collection, Field, Record)
│   ├── database/             # pgxpool setup, dual-pool isolation, physical DDL compiler
│   ├── query/                # AST parser, SQL compiler, FLS projection, Dataloader
│   ├── command/              # Atomic mutation pipeline, optimistic locking, audit diffing
│   ├── queue/                # SKIP LOCKED job queue worker & crash reaper
│   ├── api/                  # HTTP router (chi), JWT auth, SSE subscription hub
│   └── config/               # Environment & flag parsing
├── web/                      # Svelte 5 Admin SPA source code
├── templates/                # Reference JSON manifests (crm.json, commerce.json)
├── tests/
│   ├── integration/          # Real PostgreSQL integration tests
│   └── benchmarks/           # Throughput and allocation benchmarks
├── docs/                     # Production architectural & API documentation
├── Makefile                  # FAANG standard build & verification targets
└── kilo.json                 # Project configuration for Kilo AI
```

### Layer Rules:
- `internal/core` must **never import** database drivers, HTTP routers, or external packages.
- Read queries must execute via `internal/query` (Query Path).
- Write mutations must execute via `internal/command` (Command Path).
- Database operations must be split:
  - **Control Pool**: Schema DDL, migrations, tenant provisioning (max 5 conns, user `modula_control`).
  - **Data Pool**: Runtime DML (`SELECT`, `INSERT`, `UPDATE`, `DELETE`) with zero DDL privileges (user `modula_runtime`).

---

## 3. Concurrency, Locking & Transactions

1. **Optimistic Concurrency Control**:
   - Every mutation checks the `version` column:
     ```sql
     UPDATE usr_orders 
     SET status = $1, version = version + 1, updated_at = now(), updated_by = $2
     WHERE public_id = $3 AND version = $4 AND deleted_at IS NULL;
     ```
   - If 0 rows are affected, return `ErrVersionConflict` (mapped to HTTP 409 Conflict).

2. **Atomic Audit Diffing**:
   - Audit diffs and record mutations must occur inside a single atomic transaction or CTE. Never compute diffs asynchronously from uncommitted memory state.

3. **Bounded DDL Execution**:
   - Dynamic schema changes must execute with tight timeouts:
     ```sql
     SET LOCAL lock_timeout = '2s';
     SET LOCAL statement_timeout = '5s';
     ```
   - Implement exponential retry backoff (3 attempts) on lock acquisition failure.

4. **Job Queue Worker Semantics**:
   - Use atomic `FOR UPDATE SKIP LOCKED`.
   - Always record `locked_at = now()`.
   - The reaper daemon must periodically reset orphaned jobs (`status = 'RUNNING' AND locked_at < now() - INTERVAL '5 minutes'`).

---

## 4. Coding Conventions & Code Hygiene

1. **Idiomatic Go**:
   - All errors must be wrapped with contextual information: `fmt.Errorf("execute statement: %w", err)`.
   - Never ignore returned errors.
   - Structured logging using standard library `log/slog`. No unstructured `fmt.Println` or `log.Printf`.
   - Goroutine lifecycle must be tied to `context.Context`. Clean shutdown on `SIGINT`/`SIGTERM`.

2. **No AI Slop in Comments**:
   - Do not write trivial or generic comments (e.g., `// constructor for Collection`, `// check if err is not nil`).
   - Comments must explain **why** non-obvious logic exists (e.g., why a specific lock level or CTE is required).
   - Never leave conversational comments directed at users or other agents.

---

## 5. Verification & Testing Protocol

Before marking any task complete or suggesting review, run the quality gate:

```bash
make verify
```

If making database or DDL changes:
1. Verify with table-driven unit tests.
2. Ensure `go test -race` passes with zero race warnings.
3. Test schema migrations with dry-run rollbacks.

---

<!-- antislop:start -->
## 6. Anti-Slop Discipline & Production Guidelines

For code comments, backend implementation, or UI engineering, consult the dedicated standards:
- **Go Standards & Comment Hygiene**: `docs/development/coding-guidelines.md`
- **Svelte 5 Admin UI & Visual Standards**: `docs/development/ui-design-guidelines.md`
- **Core Anti-Slop Rules**: `~/.kilo/skills/antislop/SKILL.md`
  - UI / visual: `~/.kilo/skills/antislop-ui/SKILL.md`
  - Code comments: `~/.kilo/skills/antislop-code/SKILL.md`
  - People & Accessibility: `~/.kilo/skills/antislop-human/SKILL.md`
  - Copy & text: `~/.kilo/skills/antislop-copywriting/SKILL.md`
<!-- antislop:end -->

