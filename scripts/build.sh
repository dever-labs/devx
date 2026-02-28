#!/usr/bin/env sh
set -e

mkdir -p dist

LDFLAGS="-s -w"
[ -n "$VERSION" ] && LDFLAGS="$LDFLAGS -X main.version=$VERSION"

GOOS=linux  GOARCH=amd64 go build -ldflags="$LDFLAGS" -o dist/devx-linux-amd64   ./cmd/devx
GOOS=linux  GOARCH=arm64 go build -ldflags="$LDFLAGS" -o dist/devx-linux-arm64   ./cmd/devx
GOOS=darwin GOARCH=amd64 go build -ldflags="$LDFLAGS" -o dist/devx-darwin-amd64  ./cmd/devx
GOOS=darwin GOARCH=arm64 go build -ldflags="$LDFLAGS" -o dist/devx-darwin-arm64  ./cmd/devx
GOOS=windows GOARCH=amd64 go build -ldflags="$LDFLAGS" -o dist/devx-windows-amd64.exe ./cmd/devx
GOOS=windows GOARCH=arm64 go build -ldflags="$LDFLAGS" -o dist/devx-windows-arm64.exe ./cmd/devx

echo "Built:"
ls -lh dist/
