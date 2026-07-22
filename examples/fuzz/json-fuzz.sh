#!/usr/bin/env bash
# JSON body fuzzing
searchit fuzz \
  -u http://example.com/api/login \
  -X POST \
  -d '{"username":"admin","password":"FUZZ"}' \
  -H 'Content-Type: application/json' \
  -w passwords.txt
