# Storage Engine & Dynamic DDL Compiler

**Status**: Production Specification  
**Version**: 1.0.0-MVP  

---

## 1. Dynamic Table Model (`usr_*`)

User collections map directly to physical PostgreSQL tables prefixed with `usr_`. Every collection table is created with invariant physical columns:

```sql
CREATE TABLE usr_<collection> (
    -- Internal Primary Key
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    -- Public Identifier (Unique per tenant)
    public_id UUID NOT NULL DEFAULT gen_random_uuid(),

    -- Tenant Isolation
    tenant_id BIGINT NOT NULL REFERENCES sys_tenants(id) ON DELETE CASCADE,

    -- Audit Metadata
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by BIGINT NOT NULL REFERENCES sys_users(id),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by BIGINT NOT NULL REFERENCES sys_users(id),

    -- Soft Deletion & Concurrency
    deleted_at TIMESTAMPTZ NULL,
    deleted_by BIGINT NULL REFERENCES sys_users(id),
    version INT NOT NULL DEFAULT 1,

    -- Composite Uniqueness for Cross-Tenant FK Integrity
    CONSTRAINT uq_<collection>_tenant_id UNIQUE (tenant_id, id),
    CONSTRAINT uq_<collection>_tenant_public_id UNIQUE (tenant_id, public_id)
);

-- Core Operational Indexes
CREATE INDEX idx_<collection>_tenant_active ON usr_<collection>(tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_<collection>_public_lookup ON usr_<collection>(tenant_id, public_id) WHERE deleted_at IS NULL;
```

---

## 2. Field Type Mappings

| Logical Field Type | PostgreSQL Physical Type | Default Constraint | Index Strategy |
|---|---|---|---|
| `text` | `TEXT` or `VARCHAR(N)` | `NULL` | B-Tree (if `searchable`), Trigram GIN (if text search enabled) |
| `number` | `NUMERIC(precision, scale)` or `BIGINT` | `DEFAULT 0` | B-Tree |
| `boolean` | `BOOLEAN` | `DEFAULT false` | Partial B-Tree (only if low cardinality filter) |
| `datetime` | `TIMESTAMPTZ` | `DEFAULT now()` | B-Tree |
| `enum` | `VARCHAR(64)` | `CHECK (field IN (...))` | B-Tree |
| `json` | `JSONB` | `DEFAULT '{}'::jsonb` | GIN (`jsonb_path_ops`) |
| `relation (M2O)` | `BIGINT` | `REFERENCES usr_<target>(tenant_id, id)` | B-Tree |

---

## 3. Transactional DDL Safety & Lock Management

Dynamic schema changes run on the **Control Pool** inside explicit transaction blocks:

```sql
BEGIN;
SET LOCAL lock_timeout = '2s';
SET LOCAL statement_timeout = '5s';

-- 1. Execute DDL statement
ALTER TABLE usr_orders ADD COLUMN notes TEXT NULL;

-- 2. Update system catalog
INSERT INTO sys_fields (collection_id, name, type, is_nullable)
VALUES ($1, 'notes', 'text', true);

COMMIT;
```

### 3.1 Lock Contention Mitigation
1. **Short Lock Wait**: `lock_timeout = '2s'` guarantees DDL aborts if a concurrent query holds an incompatible lock, preventing queue pileups.
2. **Exponential Backoff**: Failed DDL operations retry up to 3 times with randomized exponential backoff (100ms, 400ms, 1200ms).
3. **Pre-flight Type Cast Validation**: Before modifying column types, the engine executes a non-blocking check:
   ```sql
   SELECT 1 FROM usr_orders 
   WHERE notes !~ '^[0-9]+$' 
   LIMIT 1;
   ```
   If violating rows exist, the migration is rejected with HTTP 422 Unprocessable Entity before any `ALTER TABLE` is issued.

---

## 4. Non-Destructive Column Deprecation (Tombstoning)

Dropping a column through the API or UI never issues an immediate `ALTER TABLE DROP COLUMN`. Immediate drops risk downtime and lock tables.

### Tombstoning Procedure:
1. In `sys_fields`, mark field `is_active = false`.
2. In-memory metadata registry invalidates cache: query builder immediately excludes field from FLS and projections.
3. Physical column renamed inside a low-lock transaction:
   ```sql
   ALTER TABLE usr_orders RENAME COLUMN legacy_field TO tombstone_legacy_field_1730700000;
   ```
4. Background vacuum worker permanently purges tombstoned columns after a configurable retention window (default: 30 days).
