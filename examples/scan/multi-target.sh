#!/usr/bin/env bash
# Scan multiple targets — comma separated
searchit scan -u http://example.com,http://example.org -w words.txt

# Scan multiple targets from a file (one URL per line)
searchit scan --url-file targets.txt -w words.txt
