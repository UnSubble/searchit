#!/usr/bin/env bash
set -euo pipefail

echo "=== Running Stress & Race Safety Tests ==="

CORE_PKGS="./cmd ./internal/engine ./internal/fuzz ./internal/recursion ./internal/adaptive ./internal/output ./internal/wordlist ./internal/filter ./internal/status ./internal/size ./internal/robots ./internal/sitemap ./internal/stats ./internal/requesttemplate ./internal/targets"

# 1. Race Detector Test
echo "[1/3] Running go test -race..."
go test -race ${CORE_PKGS}
echo "  ✓ Data race safety verified (0 race conditions detected)"

# 2. Shuffle Order Test
echo "[2/3] Running go test -shuffle=on (multiple iterations)..."
go test -shuffle=on -count=2 ${CORE_PKGS}
echo "  ✓ Test shuffle stability verified"

# 3. High Count Stress Test
echo "[3/3] Running go test -count=5 on core pipeline packages..."
go test -count=5 ./internal/engine ./internal/fuzz ./internal/recursion ./internal/adaptive
echo "  ✓ High iteration stress tests passed"

echo "=== Stress & Race Safety Passed ==="
