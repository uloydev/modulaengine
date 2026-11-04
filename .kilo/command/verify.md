---
description: Run full quality gate verification (lint, staticcheck, and race-detected tests)
agent: qa-engineer
---
Execute the ModulaEngine pre-commit quality gate:

1. Run `make lint` or `go vet ./...` to ensure clean static analysis.
2. Run `make test-race` (`go test -v -race -short ./...`) to verify tests with zero data race warnings.
3. If any failure occurs, diagnose the root cause, fix the issue, and re-run until all checks pass cleanly.
