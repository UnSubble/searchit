#!/usr/bin/env bash
# Multi-placeholder fuzzing — FUZZ + FOO
# Tests all combinations: each FUZZ word x each FOO word
searchit fuzz \
  -u 'http://example.com/FUZZ/FOO' \
  -w sections.txt \
  --foo pages.txt \
  --strategy bfs

# Three placeholders: FUZZ + FOO + BAR
searchit fuzz \
  -u 'http://example.com/api/FUZZ/FOO/BAR' \
  -w versions.txt \
  --foo endpoints.txt \
  --bar params.txt
