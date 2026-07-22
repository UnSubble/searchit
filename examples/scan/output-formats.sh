#!/usr/bin/env bash
# Output as JSON (auto-detected from .json extension)
searchit scan -u http://example.com -w words.txt -o results.json

# Output as CSV
searchit scan -u http://example.com -w words.txt -o results.csv

# Output as Markdown table
searchit scan -u http://example.com -w words.txt -o results.md

# Explicit format on stdout
searchit scan -u http://example.com -w words.txt --format ndjson

# Show response headers and HTML title
searchit scan -u http://example.com -w words.txt --show-headers --show-title -o results.json
