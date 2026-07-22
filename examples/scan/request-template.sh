#!/usr/bin/env bash
# Scan using a raw HTTP request template file
searchit scan -u http://example.com --request requests/get.http -w words.txt
