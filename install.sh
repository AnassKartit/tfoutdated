#!/usr/bin/env bash
set -euo pipefail

REPO="anasskartit/tfoutdated"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
esac

VERSION="${1:-latest}"
if [ "$VERSION" = "latest" ]; then
  VERSION=$(curl -sI "https://github.com/${REPO}/releases/latest" | grep -i location | sed 's/.*tag\///' | tr -d '\r\n')
fi

echo "Installing tfoutdated ${VERSION} (${OS}/${ARCH})..."

URL="https://github.com/${REPO}/releases/download/${VERSION}/tfoutdated_${VERSION#v}_${OS}_${ARCH}.tar.gz"
TMP=$(mktemp -d)

curl -sL "$URL" -o "${TMP}/tfoutdated.tar.gz"
tar -xzf "${TMP}/tfoutdated.tar.gz" -C "${TMP}"

if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP}/tfoutdated" "${INSTALL_DIR}/tfoutdated"
else
  sudo mv "${TMP}/tfoutdated" "${INSTALL_DIR}/tfoutdated"
fi

rm -rf "$TMP"

echo "tfoutdated installed to ${INSTALL_DIR}/tfoutdated"
tfoutdated version
