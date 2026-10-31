#!/usr/bin/env bash
# Dev Container Feature install script for devx.
# https://containers.dev/implementors/features/
#
# Feature options are passed in as environment variables matching the
# option's id, upper-cased (see devcontainer-feature.json): VERSION.
set -e

REPO="dever-labs/devx"
BINARY="devx"
INSTALL_DIR="/usr/local/bin"
VERSION="${VERSION:-latest}"

echo "Activating feature 'devx'"

if ! command -v curl >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -y
    apt-get install -y --no-install-recommends curl ca-certificates
  elif command -v apk >/dev/null 2>&1; then
    apk add --no-cache curl ca-certificates
  else
    echo "curl is required to install devx but could not be installed automatically." >&2
    exit 1
  fi
fi

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac
case "$OS" in
  linux) ;;
  *) echo "Unsupported OS for this Feature: $OS" >&2; exit 1 ;;
esac
PLATFORM="${OS}-${ARCH}"

if [ "$VERSION" = "latest" ]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
fi

ASSET="${BINARY}-${PLATFORM}"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

echo "Downloading devx ${VERSION} for ${PLATFORM}..."
TMP="$(mktemp)"
curl -fsSL "$URL" -o "$TMP"

# Verify against the release's published checksums.txt so a tampered/MITM'd
# download is caught before the binary is installed and executed.
CHECKSUMS_TMP="$(mktemp)"
if ! curl -fsSL "$CHECKSUMS_URL" -o "$CHECKSUMS_TMP"; then
  echo "Failed to download checksums.txt for verification; aborting." >&2
  exit 1
fi
EXPECTED="$(grep " ${ASSET}\$" "$CHECKSUMS_TMP" | awk '{print $1}')"
rm -f "$CHECKSUMS_TMP"
if [ -z "$EXPECTED" ]; then
  echo "No checksum entry found for ${ASSET}; aborting." >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL="$(sha256sum "$TMP" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL="$(shasum -a 256 "$TMP" | awk '{print $1}')"
else
  echo "Neither sha256sum nor shasum is available; cannot verify checksum, aborting." >&2
  exit 1
fi
if [ "$EXPECTED" != "$ACTUAL" ]; then
  echo "Checksum mismatch for ${ASSET}: expected ${EXPECTED}, got ${ACTUAL}" >&2
  exit 1
fi

chmod +x "$TMP"
mv "$TMP" "${INSTALL_DIR}/${BINARY}"

echo "Installed devx ${VERSION} to ${INSTALL_DIR}/${BINARY}"
"${INSTALL_DIR}/${BINARY}" version || true
