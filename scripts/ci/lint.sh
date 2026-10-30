#!/usr/bin/env bash
set -euo pipefail

echo "=== Running Lint & Static Analysis ==="

# 1. Format Check
if [[ "${1:-}" != "--audit" && "${1:-}" != "--skip-fmt" ]]; then
    echo "[1/3] Checking gofmt formatting..."
    unformatted=$(gofmt -l .)
    if [ -n "${unformatted}" ]; then
        echo "ERROR: The following files are not formatted cleanly:"
        echo "${unformatted}"
        exit 1
    fi
    echo "  ✓ gofmt passed"
fi

# 2. Go Vet
echo "[2/3] Running go vet..."
go vet ./...
echo "  ✓ go vet passed"

# 3. Staticcheck
echo "[3/3] Running staticcheck..."
STATICCHECK="$(go env GOPATH)/bin/staticcheck"
if ! command -v "${STATICCHECK}" &> /dev/null && ! which staticcheck &> /dev/null; then
    echo "Installing staticcheck..."
    # v0.7.0 (latest release) does not support Go 1.27 export data (version 4).
    # Use master until the next tagged release ships with Go 1.27 support.
    # TODO: go install honnef.co/go/tools/cmd/staticcheck@latest
    go install honnef.co/go/tools/cmd/staticcheck@master
fi

# Staticcheck relies on golang.org/x/tools which supports Unified IR export data up to version 4 (Go 1.26).
# Go 1.27+ emits export data version 5, which staticcheck cannot decode until upstream adds V5 support:
#   "cannot decode ..., export data version 5 is greater than maximum supported version 4"
# When running under Go 1.27+, use a compatible cached Go 1.26 toolchain if available.
if [[ -z "${GOTOOLCHAIN:-}" ]]; then
    go_ver=$(go env GOVERSION)
    if [[ "${go_ver}" =~ ^go1\.([2-9][7-9]|[3-9][0-9]) ]]; then
        modcache=$(go env GOMODCACHE)
        tc_dir=$(ls -d "${modcache}"/golang.org/toolchain@*go1.26* 2>/dev/null | sort -V | tail -n 1 || true)
        if [ -n "${tc_dir}" ]; then
            tc_ver=$(basename "${tc_dir}" | sed -E "s/toolchain@v[0-9.]+-//; s/\.[^.]+$//")
            export GOTOOLCHAIN="${tc_ver}"
        fi
    fi
fi

if command -v staticcheck &> /dev/null; then
    staticcheck ./...
else
    "${STATICCHECK}" ./...
fi
echo "  ✓ staticcheck passed"

echo "=== Lint & Code Quality Passed ==="
