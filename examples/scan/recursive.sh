#!/usr/bin/env bash
# Recursive scan with HTML link extraction up to depth 3
searchit scan -u http://example.com -w words.txt -r --max-depth 3
