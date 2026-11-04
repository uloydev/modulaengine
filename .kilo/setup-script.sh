#!/usr/bin/env bash
set -euo pipefail

echo "=== Initializing ModulaEngine Agent Workspace ==="

# Copy .env from .env.example if missing
if [ ! -f "${WORKTREE_PATH:-.}/.env" ] && [ -f "${REPO_PATH:-.}/.env.example" ]; then
    echo "Creating .env from .env.example..."
    cp "${REPO_PATH:-.}/.env.example" "${WORKTREE_PATH:-.}/.env"
fi

# Ensure Go toolchain is available
if command -v go >/dev/null 2>&1; then
    echo "Go version: $(go version)"
    go mod download || true
else
    echo "WARNING: go toolchain not found in PATH"
fi

echo "=== Workspace Setup Complete ==="
