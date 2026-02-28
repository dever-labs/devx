#!/usr/bin/env sh
# Install devx — usage: curl -fsSL https://raw.githubusercontent.com/dever-labs/dever/main/scripts/install.sh | sh
set -e

REPO="dever-labs/dever"
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

  TMP="$(mktemp)"
  echo "Downloading devx ${VERSION} for ${PLATFORM}..."
  curl -fsSL "$URL" -o "$TMP"
  chmod +x "$TMP"

  if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP" "${INSTALL_DIR}/${BINARY}"
  else
    sudo mv "$TMP" "${INSTALL_DIR}/${BINARY}"
  fi
  echo "Installed devx ${VERSION} to ${INSTALL_DIR}/${BINARY}"
}

detect_platform
download_and_install "${1:-}"
