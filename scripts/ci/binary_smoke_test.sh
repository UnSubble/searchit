#!/usr/bin/env bash
set -euo pipefail

echo "=== Running Binary Smoke Tests ==="

BIN="dist/searchit"
if [ ! -f "${BIN}" ]; then
    chmod +x build.sh
    ./build.sh
fi

echo "[1/4] Testing ${BIN} --version..."
ver_out=$("${BIN}" --version)
echo "  Output: ${ver_out}"
if ! echo "${ver_out}" | grep -qi "searchit"; then
    echo "ERROR: Version output invalid"
    exit 1
fi
echo "  ✓ Version command passed"

echo "[2/4] Testing ${BIN} scan --help..."
scan_help=$("${BIN}" scan --help)
if ! echo "${scan_help}" | grep -q "scan"; then
    echo "ERROR: scan --help output invalid"
    exit 1
fi
echo "  ✓ scan --help passed"

echo "[3/4] Testing ${BIN} fuzz --help..."
fuzz_help=$("${BIN}" fuzz --help)
if ! echo "${fuzz_help}" | grep -q "fuzz"; then
    echo "ERROR: fuzz --help output invalid"
    exit 1
fi
echo "  ✓ fuzz --help passed"

echo "[4/4] Testing profile loading (${BIN} scan --profile quick)..."
quick_scan=$("${BIN}" scan --profile quick -u http://localhost -w /dev/null --no-progress 2>&1 || true)
if ! echo "${quick_scan}" | grep -qi "scan/quick"; then
    echo "ERROR: Profile quick failed to load in scan mode. Output:"
    echo "${quick_scan}"
    exit 1
fi
echo "  ✓ Profile quick scan loading verified"

echo "=== All Binary Smoke Tests Passed ==="
