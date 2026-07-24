#!/usr/bin/env bash

set -euo pipefail

VERSION="${VERSION:-$(git describe --tags --exact-match 2>/dev/null || echo "dev")}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo "dev")}"
DATE="${DATE:-$(date -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || echo "unknown")}"

LDFLAGS="-s -w \
-X github.com/unsubble/searchit/internal/version.Version=${VERSION} \
-X github.com/unsubble/searchit/internal/version.Commit=${COMMIT} \
-X github.com/unsubble/searchit/internal/version.Date=${DATE}"

echo "Cleaning previous build artifacts..."
rm -rf dist
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

	GOOS="${goos}" GOARCH="${goarch}" go build \
		-trimpath \
		-ldflags="${LDFLAGS}" \
		-o "dist/${filename}" \
		.
done

#
# Local convenience binary.
# This binary is NOT considered a release asset.
#

host_goos="$(go env GOOS)"
host_goarch="$(go env GOARCH)"

host_filename="searchit-${host_goos}-${host_goarch}"

if [ "${host_goos}" = "windows" ]; then
	host_filename="${host_filename}.exe"
fi

if [ -f "dist/${host_filename}" ]; then
	echo "Creating local binary..."

	if [ "${host_goos}" = "windows" ]; then
		cp "dist/${host_filename}" "dist/searchit.exe"
	else
		cp "dist/${host_filename}" "dist/searchit"
		chmod +x "dist/searchit"
	fi
fi

#
# Generate release checksums.
#
# The local convenience binary is intentionally excluded.
#

echo "Generating SHA256 checksums..."

(
	cd dist

	sha256sum \
		searchit-linux-amd64 \
		searchit-linux-arm64 \
		searchit-darwin-amd64 \
		searchit-darwin-arm64 \
		searchit-windows-amd64.exe \
		> checksums.txt
)

echo "Verifying checksums..."

(
	cd dist
	sha256sum -c checksums.txt
)

echo
echo "Build complete."
echo
echo "Release assets:"
echo

ls -lh \
	dist/searchit-linux-amd64 \
	dist/searchit-linux-arm64 \
	dist/searchit-darwin-amd64 \
	dist/searchit-darwin-arm64 \
	dist/searchit-windows-amd64.exe \
	dist/checksums.txt

echo
echo "Local binary:"
echo

if [ -f "dist/searchit" ]; then
	ls -lh dist/searchit
elif [ -f "dist/searchit.exe" ]; then
	ls -lh dist/searchit.exe
fi

echo
echo "Output directory: dist/"