#!/usr/bin/env bash
set -euo pipefail

mkdir -p compatibility

REPORT="compatibility/compatibility.md"
echo "# Compatibility Validation Report" > "${REPORT}"
echo "" >> "${REPORT}"
echo "Generated at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")" >> "${REPORT}"
echo "" >> "${REPORT}"
echo "## Areas Tested" >> "${REPORT}"
echo "1. Go Compiler & Runtime Version: $(go version)" >> "${REPORT}"
echo "2. Output Formatter Schemas (text, json, ndjson, csv, markdown)" >> "${REPORT}"
echo "3. Predefined Scan/Fuzz Profile Compatibility" >> "${REPORT}"
echo "4. Profile Schema v1 Validation" >> "${REPORT}"
echo "" >> "${REPORT}"

echo "=== Running Compatibility Tests ==="

echo "[1/3] Running output formatters compatibility & golden tests..."
go test -v ./internal/output -run "Golden|Format|Fuzz" | tee -a compatibility/output_formatters.log

echo "[2/3] Running profile resolver & schema compatibility tests..."
go test -v ./internal/profile/... | tee -a compatibility/profiles.log

echo "[3/3] Running CLI profile integration compatibility tests..."
go test -v ./cmd -run "Profile" | tee -a compatibility/cli_profiles.log

echo "## Status" >> "${REPORT}"
echo "**PASSED**: All output formatters (text, json, ndjson, csv, markdown) and profiles (scan/base, scan/quick, fuzz/base, fuzz/quick) adhere strictly to schema specifications." >> "${REPORT}"

echo "=== Compatibility Tests Passed ==="
