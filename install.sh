#!/usr/bin/env sh
# GoCode installer for macOS and Linux.
# Usage: curl -fsSL https://raw.githubusercontent.com/saurabh-sol/cli/main/install.sh | sh
set -e
REPO="saurabh-sol/cli"
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac
case "$OS" in darwin|linux) ;; *) echo "Unsupported OS: $OS (use install.ps1 on Windows)"; exit 1 ;; esac

TAG=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
[ -n "$TAG" ] || { echo "Could not determine latest release"; exit 1; }
URL="https://github.com/$REPO/releases/download/$TAG/gocode_${OS}_${ARCH}.tar.gz"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
echo "Downloading GoCode $TAG for $OS/$ARCH..."
curl -fsSL "$URL" -o "$TMP/gocode.tar.gz"
tar -xzf "$TMP/gocode.tar.gz" -C "$TMP"

DEST=/usr/local/bin
if [ ! -w "$DEST" ]; then DEST="$HOME/.local/bin"; fi
mkdir -p "$DEST"
install -m 755 "$TMP/gocode" "$DEST/gocode"
echo "Installed: $DEST/gocode"
case ":$PATH:" in
  *":$DEST:"*) ;;
  *) echo "Add $DEST to your PATH, e.g.:  export PATH=\"$DEST:\$PATH\"" ;;
esac
echo "Run: gocode"
