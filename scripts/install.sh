#!/usr/bin/env sh
# Install devx — usage: curl -fsSL https://raw.githubusercontent.com/dever-labs/devx/main/scripts/install.sh | sh
set -e

REPO="dever-labs/devx"
BINARY="devx"
INSTALL_DIR="${DEVX_INSTALL_DIR:-/usr/local/bin}"

detect_platform() {
  OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
  ARCH="$(uname -m)"
  case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
  esac
  case "$OS" in
    linux|darwin) ;;
    *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
  esac
  PLATFORM="${OS}-${ARCH}"
}

latest_version() {
  curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/'
}

download_and_install() {
  VERSION="${1:-$(latest_version)}"
  ASSET="${BINARY}-${PLATFORM}"
  URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"
  CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

  TMP="$(mktemp)"
  echo "Downloading devx ${VERSION} for ${PLATFORM}..."
  curl -fsSL "$URL" -o "$TMP"
  verify_checksum "$TMP" "$ASSET" "$CHECKSUMS_URL"
  chmod +x "$TMP"

  if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP" "${INSTALL_DIR}/${BINARY}"
  else
    sudo mv "$TMP" "${INSTALL_DIR}/${BINARY}"
  fi
  echo "Installed devx ${VERSION} to ${INSTALL_DIR}/${BINARY}"
}

# verify_checksum downloads the release's checksums.txt and confirms the
# downloaded binary's SHA-256 matches, so a compromised/tampered download
# (e.g. a MITM'd mirror or CDN) is caught before the binary is installed
# and executed.
verify_checksum() {
  file="$1"
  asset="$2"
  checksums_url="$3"

  checksums_tmp="$(mktemp)"
  if ! curl -fsSL "$checksums_url" -o "$checksums_tmp"; then
    rm -f "$checksums_tmp"
    echo "Failed to download checksums.txt for verification; aborting." >&2
    exit 1
  fi

  expected="$(grep " ${asset}\$" "$checksums_tmp" | awk '{print $1}')"
  rm -f "$checksums_tmp"
  if [ -z "$expected" ]; then
    echo "No checksum entry found for ${asset}; aborting." >&2
    exit 1
  fi

  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$file" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$file" | awk '{print $1}')"
  else
    echo "Neither sha256sum nor shasum is available; cannot verify checksum, aborting." >&2
    exit 1
  fi

  if [ "$expected" != "$actual" ]; then
    echo "Checksum mismatch for ${asset}: expected ${expected}, got ${actual}" >&2
    exit 1
  fi
}

detect_platform
download_and_install "${1:-}"
