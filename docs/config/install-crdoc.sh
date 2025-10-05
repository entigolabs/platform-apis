#!/bin/bash

set -e

# Default version
VERSION="${CRDOC_VERSION:-0.6.4}"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
    darwin)
        OS="darwin"
        ;;
    linux)
        OS="linux"
        ;;
    *)
        echo "Unsupported OS: $OS"
        exit 1
        ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64 | amd64)
        ARCH="amd64"
        ;;
    aarch64 | arm64)
        ARCH="arm64"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

echo "Detected OS: $OS"
echo "Detected Architecture: $ARCH"
echo "Version: $VERSION"

GITHUB_REPO="fybrik/crdoc"
PACKAGE_NAME="crdoc"

FILENAME="${PACKAGE_NAME}_${OS}_${ARCH}.tar.gz"

DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}/${FILENAME}"

echo "Downloading from: $DOWNLOAD_URL"

# Download the package
if command -v curl > /dev/null 2>&1; then
    curl -L -o "$FILENAME" "$DOWNLOAD_URL"
elif command -v wget > /dev/null 2>&1; then
    wget -O "$FILENAME" "$DOWNLOAD_URL"
else
    echo "Error: neither curl nor wget is available"
    exit 1
fi

echo "Downloaded: $FILENAME"

tar -xzf "$FILENAME"
echo "Extraction complete"

echo "Download complete!