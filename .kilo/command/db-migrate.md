---
description: Validate and dry-run PostgreSQL schema migrations and DDL statements
agent: database-specialist
---
Verify and test database migrations for ModulaEngine:

1. Inspect all pending schema changes and DDL definitions.
2. Confirm strict adherence to storage invariants:
   - Dynamic tables prefixed with `usr_`
   - Dual identifiers: `id BIGINT GENERATED ALWAYS AS IDENTITY` and `public_id UUID`
   - Composite keys: `UNIQUE (tenant_id, id)` and `FOREIGN KEY (tenant_id, fk_id) REFERENCES usr_target(tenant_id, id)`
   - Safe DDL lock timeouts: `SET LOCAL lock_timeout = '2s'` and `SET LOCAL statement_timeout = '5s'`
   - Non-destructive deprecation (tombstoning) for dropped columns
3. Execute dry-run migration validation against target PostgreSQL instance.
