---
description: Perform multi-axis architectural and security code review against ModulaEngine invariants
agent: architect
---
Perform a FAANG-grade code review across all changed files:

1. Architectural Invariants Check:
   - Zero domain-specific logic in Go core packages (Anti-Overfitting).
   - Single binary and strict memory boundaries maintained.
   - Dual-identifier paradigm strictly respected (no `id BIGINT` in API outputs).
2. Security & Multi-Tenancy Audit:
   - Root AST `tenant_id = $tenant_id` predicate injection present on all queries.
   - Composite foreign keys enforced on all relational joins.
   - Zero dynamic SQL concatenation; 100% parameter binding ($1, $2, ...).
3. Concurrency & Reliability Review:
   - Optimistic concurrency control via `version` column checks.
   - Atomic audit diffing executed in single CTE / transaction.
   - Context propagation and goroutine leak prevention.
4. Report clear findings and block any changes violating these invariants.
