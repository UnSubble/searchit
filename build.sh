#!/usr/bin/env bash

set -euo pipefail

VERSION="${VERSION:-$(git describe --tags --exact-match 2>/dev/null || echo "v0.3.0-alpha")}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo "dev")}"
DATE="${DATE:-$(date -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || echo "unknown")}"

LDFLAGS="-s -w -X github.com/unsubble/searchit/internal/version.Version=${VERSION} -X github.com/unsubble/searchit/internal/version.Commit=${COMMIT} -X github.com/unsubble/searchit/internal/version.Date=${DATE}"

mkdir -p dist

targets=(
	"linux/amd64/searchit-linux-amd64"
	"linux/arm64/searchit-linux-arm64"
	"windows/amd64/searchit-windows-amd64.exe"
	"darwin/amd64/searchit-darwin-amd64"
	"darwin/arm64/searchit-darwin-arm64"
)

echo "Building target binaries..."

for t in "${targets[@]}"; do
	IFS="/" read -r goos goarch filename <<< "$t"
	echo "  - ${goos}/${goarch} -> dist/${filename}"
	GOOS=${goos} GOARCH=${goarch} go build \
		-trimpath \
		-ldflags="${LDFLAGS}" \
		-o "dist/${filename}" \
		.
done

# Copy host architecture binary to dist/searchit for local use and quickstart compatibility
host_goos="$(go env GOOS)"
host_goarch="$(go env GOARCH)"
host_filename="searchit-${host_goos}-${host_goarch}"
if [ "${host_goos}" = "windows" ]; then
	host_filename="${host_filename}.exe"
fi

if [ -f "dist/${host_filename}" ]; then
	echo "Creating local binary dist/searchit -> dist/${host_filename}"
	if [ "${host_goos}" = "windows" ]; then
		cp "dist/${host_filename}" "dist/searchit.exe"
	else
		cp "dist/${host_filename}" "dist/searchit"
		chmod +x "dist/searchit"
	fi
fi

echo "Build complete. Output directory: dist/"
ls -lh dist/