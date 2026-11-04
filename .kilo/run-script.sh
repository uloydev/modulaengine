#!/usr/bin/env bash
set -euo pipefail

echo "=== Starting ModulaEngine Development Server ==="

PORT="${PORT:-8080}"
echo "Running on port: $PORT"

if [ -f "cmd/modulaengine/main.go" ]; then
    exec go run ./cmd/modulaengine --port="$PORT"
else
    echo "cmd/modulaengine/main.go not found yet. Running make build or placeholder check."
    exit 0
fi
