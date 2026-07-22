#!/usr/bin/env bash
# Cookie fuzzing — fuzz a session cookie value
searchit fuzz \
  -u http://example.com/dashboard \
  -b 'session=FUZZ' \
  -w sessions.txt
