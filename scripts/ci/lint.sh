#!/usr/bin/env bash
set -euo pipefail

echo "=== Running Lint & Static Analysis ==="

# 1. Format Check
echo "[1/3] Checking gofmt formatting..."
unformatted=$(gofmt -l .)
if [ -n "${unformatted}" ]; then
    echo "ERROR: The following files are not formatted cleanly:"
    echo "${unformatted}"
    exit 1
fi
echo "  ✓ gofmt passed"

# 2. Go Vet
echo "[2/3] Running go vet..."
go vet ./...
echo "  ✓ go vet passed"

# 3. Staticcheck
echo "[3/3] Running staticcheck..."
STATICCHECK="$(go env GOPATH)/bin/staticcheck"
if ! command -v "${STATICCHECK}" &> /dev/null && ! which staticcheck &> /dev/null; then
    echo "Installing staticcheck..."
    go install honnef.co/go/tools/cmd/staticcheck@v0.4.7
fi

if command -v staticcheck &> /dev/null; then
    staticcheck ./...
else
    "${STATICCHECK}" ./...
fi
echo "  ✓ staticcheck passed"

echo "=== Lint & Code Quality Passed ==="
