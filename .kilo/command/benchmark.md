---
description: Run latency and memory allocation benchmarks to audit p95 budgets and memory footprint
agent: qa-engineer
---
Execute performance benchmarks for ModulaEngine:

1. Run `make bench` (`go test -run=^$ -bench=. -benchmem ./...`).
2. Audit results against production SLAs:
   - Read latency p95 < 15ms
   - Write latency p95 < 35ms
   - Zero unexpected heap allocations in critical query compilation and AST traversal paths.
3. Compare against baseline and report any performance regressions.
