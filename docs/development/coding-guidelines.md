# ModulaEngine Engineering & Coding Guidelines

**Status**: Production Standard  
**Scope**: All Go backend code and AI agent contributions  

---

## 1. Anti-AI-Slop Code Hygiene

AI-generated code frequently suffers from conversational fluff, defensive over-commenting, and visual noise. The following rules are strictly enforced.

### 1.1 Forbidden Comment Patterns

| Pattern | Bad Example | Why Rejected | Remediation |
|---|---|---|---|
| **Decorative Separators** | `// ====================`<br>`// AUTHENTICATION`<br>`// ====================` | Visual noise signaling AI generator. | Remove entirely or use a single clean line. |
| **Restating the Obvious** | `// Initialize count`<br>`let count = 0`<br>`// User struct`<br>`type User struct` | Doubles cognitive load without adding information. | Delete comment. Code speaks for itself. |
| **Workflow Narration** | `// Step 1: Validate input`<br>`// Step 2: Query DB`<br>`// Step 3: Return JSON` | Code flow is already visible in syntax. | Delete. If flow is complex, extract into named helper functions. |
| **Empty Labels** | `// Helper function`<br>`// Business logic`<br>`// Entry point` | Categorical label carrying zero technical facts. | Delete unless specific non-obvious constraint is stated. |
| **Vague Placeholders** | `// TODO: Optimize this`<br>`// Future improvement` | Unactionable noise. | Remove or replace with concrete actionable issue and context. |
| **Signature Echo** | `// GetUser returns user by id`<br>`func GetUser(id string) (*User, error)` | Repeats the function signature verbatim. | Remove or document specific edge cases and error conditions. |
| **Decorative Emoji** | `// 🚀 Performance boost`<br>`// ✅ Validation passed` | Slop aesthetic. | Remove emoji. Use clear technical English. |
| **End Block Markers** | `} // end if`<br>`} // end for` | Redundant with modern editors and syntax. | Delete. |

### 1.2 Legitimate High-Value Comments
Comments must explain **why** something exists, not **what** the code does. Keep comments to 1–2 lines maximum. Acceptable subjects:
- **Locking & Concurrency Invariants**: Why a table lock, row lock, or specific isolation level is required.
- **Protocol & Driver Traps**: Workarounds for PostgreSQL binary protocol nuances or pgx behaviors.
- **Security Boundaries**: Justification for non-obvious sanitization or FLS masking logic.
- **Performance Trade-offs**: Why a specific data structure or memory allocation pattern was chosen.

```go
// Good: explains non-obvious protocol requirement
// pgx.CopyFrom requires columns in exact physical tuple order to prevent binary decode errors.
```

---

## 2. Go Architecture & Idiomatic Standards

### 2.1 Error Handling Invariants
1. **Contextual Error Wrapping**:
   - Always wrap lower-level errors using `%w`:
     ```go
     return fmt.Errorf("compile ast for collection %s: %w", colName, err)
     ```
   - Never use bare `return err` unless passing a sentinel error directly.
2. **Sentinel Errors & Inspection**:
   - Declare typed sentinel errors at package roots:
     ```go
     var (
         ErrNotFound        = errors.New("record not found")
         ErrVersionConflict = errors.New("optimistic concurrency conflict")
         ErrUnauthorized    = errors.New("tenant authorization failure")
     )
     ```
   - Always inspect errors using `errors.Is` and `errors.As`. Never do string comparison (`err.Error() == "..."`).
3. **Zero Discarded Errors**:
   - `_ = row.Scan(...)` or ignoring returned errors from `Close()` or `Rollback()` is prohibited.
   - Handled errors must be logged once with structured attributes via standard library `log/slog`. Never double-log and return.

### 2.2 Concurrency & Goroutine Hygiene
1. **Context Cancellation Propagation**:
   - Every I/O, database query, network request, and background worker must accept and obey `ctx context.Context`.
   - Never use `context.Background()` inside handlers or services. Always propagate caller context.
2. **Goroutine Lifecycle Bounds**:
   - Every spawned goroutine must have a deterministic termination condition tied to context cancellation or channel closure.
   - Background workers must use `sync.WaitGroup` or `errgroup.Group` for graceful shutdown on `SIGINT`/`SIGTERM`.
3. **Race Condition Prevention**:
   - Concurrent maps must use `sync.RWMutex` or `sync.Map`.
   - Every test suite must pass with `go test -race` with zero race detector warnings.

### 2.3 Memory Allocations & Performance
1. **Slice Pre-allocation**:
   - When the slice size is known or estimable, pre-allocate backing arrays:
     ```go
     items := make([]Record, 0, len(rawRows))
     ```
2. **Buffer Pooling**:
   - For high-frequency serialization (AST compilation, JSON encoding), use `sync.Pool` for `bytes.Buffer`.
3. **Zero Dynamic SQL Formatting**:
   - `fmt.Sprintf("SELECT * FROM %s WHERE id = %s", ...)` is grounds for immediate PR rejection.
   - All dynamic SQL must pass through the AST Parameterized Compiler with positional parameter binding (`$1, $2, ...`).

### 2.4 Anti-Overfitting Principle
- The Go core packages (`internal/core`, `internal/query`, `internal/command`, `internal/database`) must contain **zero domain-specific structs or terms** (no `Lead`, `Deal`, `Order`, `Product`, `Ticket`).
- ModulaEngine only operates on structural primitives: `Tenant`, `Collection`, `Field`, `Record`, `Relation`, `FilterAST`, and `AuditEvent`.
- Domain models are dynamically configured via runtime JSON manifests (`templates/*.json`).
