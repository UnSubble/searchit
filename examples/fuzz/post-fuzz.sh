#!/usr/bin/env bash
# POST body fuzzing — fuzz form fields
searchit fuzz \
  -u http://example.com/login \
  -X POST \
  -d 'username=admin&password=FUZZ' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -w passwords.txt
