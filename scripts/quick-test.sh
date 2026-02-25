#!/bin/bash

set -e

echo "=== Quick Config Test ==="

cd "$(dirname "$0")/.."

mkdir -p logs

echo "Testing YAML config loader..."
go test -v -run TestYAMLConfigLoader ./internal/test/

echo ""
echo "✅ All config loaders working!"
