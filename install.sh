#!/bin/bash
set -e

REPO="ica-js/hacktivator"
INSTALL_DIR="/usr/local/bin"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    arm64)   ARCH="arm64" ;;
    *)       echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
    darwin) OS="darwin" ;;
    linux)  OS="linux" ;;
    *)      echo "Unsupported OS: $OS"; exit 1 ;;
esac

PATTERN="hacktivator_*_${OS}_${ARCH}.tar.gz"

echo "🔍 Detected: ${OS}/${ARCH}"
echo "📦 Downloading hacktivator from ${REPO}..."

# Check if gh CLI is available
if ! command -v gh &> /dev/null; then
    echo "❌ GitHub CLI (gh) is required but not installed."
    echo "   Install it from: https://cli.github.com/"
    exit 1
fi

# Check if authenticated
if ! gh auth status &> /dev/null; then
    echo "❌ Not authenticated with GitHub CLI."
    echo "   Run: gh auth login"
    exit 1
fi

# Create temp directory
TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

# Download latest release
gh release download --repo "$REPO" --pattern "$PATTERN" --dir "$TMPDIR"

# Extract
cd "$TMPDIR"
tar -xzf hacktivator_*.tar.gz

# Install
echo "📥 Installing to ${INSTALL_DIR}/hacktivator..."
if [ -w "$INSTALL_DIR" ]; then
    mv hacktivator "$INSTALL_DIR/"
else
    sudo mv hacktivator "$INSTALL_DIR/"
fi

echo "✅ hacktivator installed successfully!"
echo ""
echo "Run 'hacktivator --help' to get started."