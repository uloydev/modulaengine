.PHONY: all build clean test test-race test-integration lint verify bench run help

# Build configuration
BINARY_NAME := modulaengine
BIN_DIR := bin
CMD_DIR := cmd/modulaengine
GO_FILES := $(shell find . -type f -name '*.go' -not -path "./vendor/*" -not -path "./web/*")

# Version and build flags
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.1.0-dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)

all: verify build

build: ## Build single self-contained binary
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) ./$(CMD_DIR)

run: build ## Run ModulaEngine binary locally
	./$(BIN_DIR)/$(BINARY_NAME) --port=8080

test: ## Run unit tests
	go test -v -cover -short ./...

test-race: ## Run unit tests with race detection
	go test -v -race -short ./...

test-integration: ## Run full integration test suite with Postgres
	go test -v -race -tags=integration ./tests/integration/...

lint: ## Run linters and static checks
	@which golangci-lint > /dev/null 2>&1 && golangci-lint run ./... || go vet ./...

verify: lint test-race ## Run lint and race-detector tests (pre-commit gate)

bench: ## Run CPU and memory allocation benchmarks
	go test -run=^$$ -bench=. -benchmem ./...

clean: ## Remove build artifacts and temporary files
	rm -rf $(BIN_DIR) *.out *.test coverage.html profile.pprof

help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
