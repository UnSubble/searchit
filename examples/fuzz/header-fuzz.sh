#!/usr/bin/env bash
# Header fuzzing — fuzz a custom request header value
searchit fuzz \
  -u http://example.com/admin \
  -H 'X-Forwarded-For: FUZZ' \
  -w ips.txt

# Fuzz an Authorization Bearer token
searchit fuzz \
  -u http://example.com/api/users \
  -H 'Authorization: Bearer FUZZ' \
  -w tokens.txt
