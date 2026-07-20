#!/usr/bin/env bash
set -euo pipefail

mkdir -p coverage

REPORT="coverage/coverage.md"
echo "# Code Coverage Report" > "${REPORT}"
echo "" >> "${REPORT}"
echo "Generated at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")" >> "${REPORT}"
echo "" >> "${REPORT}"

echo "=== Running Code Coverage Analysis ==="

go test -coverprofile=coverage.out -covermode=atomic ./... | tee coverage/coverage_raw.log

TMP_DIR=$(mktemp -d)
trap 'rm -rf "${TMP_DIR}"' EXIT

cat << 'EOF' > "${TMP_DIR}/parse_coverage.go"
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

func main() {
	covLogFile := os.Args[1]
	covReportFile := os.Args[2]

	file, err := os.Open(covLogFile)
	if err != nil {
		fmt.Printf("ERROR reading coverage log: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	re := regexp.MustCompile(`ok\s+(\S+)\s+.*coverage:\s+([\d\.]+)%\s+of\s+statements`)

	report, _ := os.OpenFile(covReportFile, os.O_APPEND|os.O_WRONLY, 0644)
	defer report.Close()

	fmt.Fprintln(report, "## Package Coverage Summary")
	fmt.Fprintln(report, "| Package | Statement Coverage |")
	fmt.Fprintln(report, "|---|---|")

	var totalCov float64
	var count int

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		matches := re.FindStringSubmatch(line)
		if len(matches) > 2 {
			pkg := matches[1]
			cov, _ := strconv.ParseFloat(matches[2], 64)
			totalCov += cov
			count++
			fmt.Fprintf(report, "| `%s` | **%.1f%%** |\n", pkg, cov)
		}
	}

	if count > 0 {
		avg := totalCov / float64(count)
		fmt.Fprintln(report, "")
		fmt.Fprintf(report, "### Overall Average Coverage: **%.1f%%**\n", avg)
	}
}
EOF

go run "${TMP_DIR}/parse_coverage.go" "coverage/coverage_raw.log" "${REPORT}"

echo "=== Code Coverage Analysis Passed ==="
