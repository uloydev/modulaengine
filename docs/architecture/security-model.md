# Security Architecture & Multi-Tenancy Specification

**Status**: Production Specification  
**Version**: 1.0.0-MVP  

---

## 1. Zero-Trust Multi-Tenancy Architecture

Multi-tenancy isolation operates on a defense-in-depth model across three distinct boundaries:

```
[ Request Ingestion ] ──> Tenant Resolution Middleware (Subdomain / Header / JWT)
                                   │
[ Query Generation  ] ──> Root AST Predicate Injection (AND tenant_id = $1)
                                   │
[ Physical Storage  ] ──> Composite DB Constraints (tenant_id, id)
```

### 1.1 Tenant Context Resolution
The HTTP middleware extracts and cryptographically validates tenant identity:
1. `Bearer` JWT claims: `tenant_id` and `user_id` signed by server secret.
2. The resolved `tenant_id` is bound to the Go `context.Context`.
3. If `tenant_id` is missing or invalid, the pipeline aborts immediately with HTTP 401 Unauthorized.

---

## 2. AST Query Compiler & Security Injection

Query generation avoids dynamic string concatenation. All SQL generation passes through an AST compiler:

### 2.1 Formula for Predicate Merging
The query compiler normalizes and compiles the final `WHERE` clause:

$$\text{Final Predicate} = (\text{deleted\_at IS NULL}) \land (\text{tenant\_id} = \$1) \land (\text{User Filter}) \land (\text{RLS Filter})$$

### 2.2 AST Safety Constraints
- **Symbol Whitelisting**: Field names, collection identifiers, and relational joins must exist in the in-memory metadata registry. Unknown identifiers trigger an immediate compilation error.
- **Strict Parameter Binding**: 100% of user-provided values are compiled to `$1, $2, ...` positional parameters.
- **Tree Depth Limit**: Maximum filter nesting depth is clamped to 5 levels to prevent AST denial-of-service.
- **Array Size Clamping**: `IN` and `NOT IN` clauses allow a maximum of 500 parameters per node.

---

## 3. Field-Level Security (FLS) & Row-Level Security (RLS)

### 3.1 Field-Level Security (FLS)
- **Read Path**: The user role's `allowed_fields` list acts as a projection whitelist. Columns not present in the whitelist are omitted from the generated `SELECT` statement.
- **Write Path**: Incoming JSON keys for `POST` and `PATCH` are compared against `allowed_fields`. Unpermitted keys are rejected with HTTP 403 Forbidden.

### 3.2 Row-Level Security (RLS)
Roles define RLS expressions stored in `sys_permissions`:
```json
{
  "op": "or",
  "children": [
    { "op": "eq", "field": "created_by", "value": "{{user.id}}" },
    { "op": "eq", "field": "is_public", "value": true }
  ]
}
```
The AST compiler replaces context variables (`{{user.id}}`, `{{user.role}}`) with verified session values and combines the expression with the user's query parameters via logical `AND`.

---

## 4. Database Connection Pool Minimization (Least Privilege)

ModulaEngine separates database operations into two isolated connection pools:

### 4.1 Control Pool (DDL / Migration)
- **Database User**: `modula_control`
- **Permissions**: `CREATE TABLE`, `ALTER TABLE`, full access to `sys_*` catalog.
- **Connection Cap**: Max 5 connections.
- **Usage**: Restricted strictly to schema migration, tenant creation, and field additions.

### 4.2 Data Pool (DML / Runtime)
- **Database User**: `modula_runtime`
- **Permissions**: `SELECT`, `INSERT`, `UPDATE`, `DELETE` on `usr_*` and selected read-only `sys_*` tables. **Zero DDL privileges**.
- **Connection Cap**: Max 25–50 connections.
- **Usage**: Handles all client API traffic. Even if an attacker achieves arbitrary SQL execution on this pool, schema alteration and privilege escalation are physically blocked by PostgreSQL role grants.

---

## 5. Cryptography & Secrets Management

- **Envelope Encryption**: Attributes marked `is_encrypted: true` are encrypted at rest with AES-256-GCM. Data Encryption Keys (DEKs) are protected by a master Key Encryption Key (KEK) passed via `MODULA_MASTER_KEY` or external KMS.
- **Password Storage**: Passwords hashed using Argon2id (`time=3`, `memory=64MB`, `threads=4`).
- **Transport Security**: TLS 1.3 enforced by default with auto-renewing ACME Let's Encrypt certificates embedded in the binary.
