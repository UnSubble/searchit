#!/usr/bin/env bash
set -euo pipefail

mkdir -p pipeline

REPORT="pipeline/pipeline.md"
echo "# Pipeline & Reconciliation Validation Report" > "${REPORT}"
echo "" >> "${REPORT}"
echo "Generated at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")" >> "${REPORT}"
echo "" >> "${REPORT}"
echo "## Stages Validated" >> "${REPORT}"
echo "1. RequestTemplate Parsing" >> "${REPORT}"
echo "2. URL Generation" >> "${REPORT}"
echo "3. Headers Injection & Formatting" >> "${REPORT}"
echo "4. Cookies Extraction & Injection" >> "${REPORT}"
echo "5. POST Bodies & Data Templating" >> "${REPORT}"
echo "6. Filtering Suite (Status, Size, Content, Regex)" >> "${REPORT}"
echo "7. Findings Dispatch & Formatter Output" >> "${REPORT}"
echo "8. End-to-End Pipeline Reconciliation" >> "${REPORT}"
echo "" >> "${REPORT}"

echo "=== Running Pipeline & Reconciliation Tests ==="

echo "[1/2] Running pipeline package unit tests..."
go test -v ./internal/requesttemplate ./internal/engine ./internal/filter ./internal/stats ./internal/output | tee -a pipeline/pipeline_units.log

echo "[2/2] Running pipeline reconciliation integration tests..."
go test -v ./cmd -run "TestIntegration|TestCLI" | tee -a pipeline/pipeline_integration.log

echo "## Status" >> "${REPORT}"
echo "**PASSED**: All pipeline stages (RequestTemplate → URLs → Headers → Cookies → POST → Filtering → Reconciliation) verified with zero mismatches." >> "${REPORT}"

echo "=== Pipeline & Reconciliation Tests Passed ==="
