#!/usr/bin/env bash

set -euo pipefail

APP_NAME="searchit"

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo none)"
DATE="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

LDFLAGS="
-X github.com/unsubble/searchit/internal/version.Version=${VERSION}
-X github.com/unsubble/searchit/internal/version.Commit=${COMMIT}
-X github.com/unsubble/searchit/internal/version.Date=${DATE}
"

mkdir -p dist

go build \
  -trimpath \
  -ldflags="-s -w ${LDFLAGS}" \
  -o dist/${APP_NAME} \
  .

echo "Built dist/${APP_NAME}"