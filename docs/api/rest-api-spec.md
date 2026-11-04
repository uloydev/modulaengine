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

```json
{
  "type": "https://modulaengine.dev/errors/concurrency-conflict",
  "title": "Optimistic Concurrency Conflict",
  "status": 409,
  "detail": "Record version 1 has been superseded. Current version is 2.",
  "instance": "/api/data/orders/018e32c8-8869-7c91-b3fa-e866184a1d52"
}
```
