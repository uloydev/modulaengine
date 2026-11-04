# REST API & Realtime Streaming Specification

**Status**: Production Specification  
**Version**: 1.0.0-MVP  

---

## 1. Dynamic Collection Endpoints (`/api/data/:collection`)

Every active collection automatically exposes uniform RESTful endpoints.

### 1.1 List Records
`GET /api/data/:collection`

#### Query Parameters
- **Filters**: `filter[<field>][<operator>]=<value>`
  - Supported operators: `_eq`, `_neq`, `_gt`, `_gte`, `_lt`, `_lte`, `_in`, `_like`, `_null`.
  - Example: `?filter[status][_eq]=PAID&filter[total_amount][_gt]=50000`
- **Sorting**: `sort=<field1>,-<field2>` (`-` indicates descending).
  - Example: `?sort=-created_at,order_number`
- **Pagination**: `limit=<n>&offset=<m>` (Default `limit=25`, max `limit=100`).
  - Example: `?limit=50&offset=100`
- **Field Projection (FLS)**: `fields=<field1>,<field2>`
  - Example: `?fields=order_number,total_amount,status`
- **Relation Expansion (Dataloader)**: `expand=<relation1>,<relation2>`
  - Resolves relational foreign keys in a single batched SQL roundtrip per relation (`WHERE id = ANY($1)`).
  - Eliminates N+1 query patterns across paginated records.
  - Automatically adheres to tenant boundaries and Field-Level Security projection masks.
  - Example: `?expand=customer,items`

#### Response Envelope (HTTP 200 OK)
```json
{
  "data": [
    {
      "id": "018e32c8-8869-7c91-b3fa-e866184a1d52",
      "order_number": "ORD-2026-001",
      "total_amount": 150000.00,
      "status": "PAID",
      "version": 1,
      "created_at": "2026-09-15T10:00:00Z"
    }
  ],
  "meta": {
    "total": 1,
    "limit": 25,
    "offset": 0
  }
}
```

---

### 1.2 Get Single Record
`GET /api/data/:collection/:public_id`

#### Response (HTTP 200 OK)
```json
{
  "data": {
    "id": "018e32c8-8869-7c91-b3fa-e866184a1d52",
    "order_number": "ORD-2026-001",
    "total_amount": 150000.00,
    "status": "PAID",
    "version": 1,
    "created_at": "2026-09-15T10:00:00Z",
    "updated_at": "2026-09-15T10:00:00Z"
  }
}
```

---

### 1.3 Create Record
`POST /api/data/:collection`

#### Request Body
```json
{
  "order_number": "ORD-2026-002",
  "customer_id": "018e32c8-8869-7c91-b3fa-e866184a1d50",
  "total_amount": 75000.00,
  "status": "DRAFT"
}
```

#### Response (HTTP 201 Created)
Returns created record including system-generated `id` (public UUID) and `version: 1`.

---

### 1.4 Update Record (Optimistic Concurrency Control)
`PATCH /api/data/:collection/:public_id`

Clients must provide the current record `version` either in the body or via `If-Match` header.

#### Request Body
```json
{
  "version": 1,
  "status": "PAID"
}
```

#### Responses
- **HTTP 200 OK**: Mutation applied successfully. Returns updated record with `version: 2`.
- **HTTP 409 Conflict**: Version mismatch detected. Mutation aborted.

---

### 1.5 Soft Delete Record
`DELETE /api/data/:collection/:public_id`

Sets `deleted_at = now()` and `deleted_by = current_user.id`.

#### Response
- **HTTP 204 No Content**

---

## 2. Realtime Subscriptions via Server-Sent Events (SSE)

`GET /api/realtime?subscribe=:collection`  
Header: `Authorization: Bearer <jwt_token>`

### Event Frame Structure
```
event: orders:update
id: 1730701200000
data: {"id":"018e32c8-8869-7c91-b3fa-e866184a1d52","action":"UPDATE","changes":{"status":"PAID"},"version":2,"updated_at":"2026-09-15T10:05:00Z"}

```

### Security Guarantees
1. SSE connections are tenant-isolated. Events never cross tenant boundaries.
2. Changes are filtered against the subscriber's role-based FLS masks in Go memory before transmission.

---

## 3. Standard Error Envelope (RFC 7807)

All non-2xx responses adhere to the RFC 7807 Problem Details specification:

```json
{
  "type": "https://modulaengine.dev/errors",
  "title": "Optimistic Concurrency Conflict",
  "status": 409,
  "detail": "optimistic concurrency conflict: expected 1, got 2",
  "instance": "/api/data/orders/018e32c8-8869-7c91-b3fa-e866184a1d52"
}
```

### Domain Error Mappings

| Domain Sentinel Error | HTTP Status Code | Problem Title |
|---|---|---|
| `core.ErrNotFound` | **404 Not Found** | Record Not Found |
| `core.ErrVersionConflict` | **409 Conflict** | Optimistic Concurrency Conflict |
| `query.ErrFieldNotPermitted` | **403 Forbidden** | Field Access Forbidden |
| `core.ErrTenantMismatch` | **401 Unauthorized** | Tenant Authorization Failure |
| `core.ErrValidation` | **400 Bad Request** | Validation Error |
| `query.ErrUnknownField` | **400 Bad Request** | Validation Error |
| `query.ErrDepthExceeded` | **400 Bad Request** | Validation Error |
| `query.ErrArraySizeExceeded` | **400 Bad Request** | Validation Error |
| Other unhandled errors | **500 Internal Server Error** | Internal Server Error |

---

## 4. System Health Probes & Runtime Metadata

Public endpoints exposed without authentication for container orchestrators and monitoring agents:

### 4.1 Liveness Probe
`GET /livez`

Validates that the HTTP listener and Go event loop are responsive. Does not perform database I/O.

#### Response (HTTP 200 OK)
```json
{
  "status": "alive"
}
```

### 4.2 Readiness Probe
`GET /readyz`

Verifies database connectivity by executing `db.Ping(ctx)` against configured connection pools.

#### Responses
- **HTTP 200 OK**: Both Data Pool and Control Pool are connected and ready to process queries.
  ```json
  {
    "status": "ready"
  }
  ```
- **HTTP 503 Service Unavailable**: Database pools are disconnected, restarting, or unreachable.
  ```json
  {
    "type": "https://modulaengine.dev/errors",
    "title": "Database Unavailable",
    "status": 503,
    "detail": "ping data pool: connection refused"
  }
  ```

### 4.3 Backward-Compatible Health Probe
`GET /healthz`

Alias endpoint forwarding to the same check and response logic as `/readyz`.

### 4.4 Version & Build Metadata
`GET /version`

Returns application version, Git commit hash, and build timestamp embedded via `-ldflags`.

#### Response (HTTP 200 OK)
```json
{
  "version": "v1.0.0-MVP",
  "commit": "a1b2c3d",
  "build_time": "2026-11-04T12:00:00Z"
}
```
