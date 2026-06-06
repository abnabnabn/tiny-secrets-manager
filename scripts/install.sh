#!/bin/bash
set -e

INSTALL_CLI=true
INSTALL_SERVER=true
DEST_DIR="/usr/local/bin"

while [[ "$#" -gt 0 ]]; do
    case $1 in
        --cli) INSTALL_SERVER=false ;;
        --server) INSTALL_CLI=false ;;
        --dest) DEST_DIR="$2"; shift ;;
        --backup-target) export TSM_BACKUP_TARGET="$2"; shift ;;
    esac
    shift
done

if [ "$INSTALL_CLI" = false ] && [ "$INSTALL_SERVER" = false ]; then
    echo "You cannot exclude both the CLI and the Server."
    exit 1
fi

echo "Ensuring directory $DEST_DIR exists and is writable..."
if ! mkdir -p "$DEST_DIR" 2>/dev/null || ! touch "$DEST_DIR/.tsm_test" 2>/dev/null; then
    echo "Error: You do not have write permissions for $DEST_DIR."
    echo "Please specify a different destination directory you have access to, or run the script as a user with appropriate permissions."
    echo "Example (custom directory): $0 --dest ~/.local/bin"
    echo "Example (with sudo):        sudo $0"
    exit 1
else
    rm -f "$DEST_DIR/.tsm_test"
fi

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux*)   OS="linux" ;;
  darwin*)  OS="darwin" ;;
  *)        echo "Unsupported OS: $OS"; exit 1 ;;
esac

ARCH=$(uname -m)
case "$ARCH" in
  x86_64)   ARCH="amd64" ;;
  amd64)    ARCH="amd64" ;;
  arm64)    ARCH="arm64" ;;
  aarch64)  ARCH="arm64" ;;
  *)        echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

SERVER_BINARY_NAME="tsm-server-${OS}-${ARCH}"
CLI_BINARY_NAME="tsm-${OS}-${ARCH}"

REPO_URL="https://github.com/abnabnabn/tiny-secrets-manager"
SERVER_GITHUB_URL="${REPO_URL}/releases/latest/download/${SERVER_BINARY_NAME}"
CLI_GITHUB_URL="${REPO_URL}/releases/latest/download/${CLI_BINARY_NAME}"

SERVER_DEST="${DEST_DIR}/tiny-secrets-manager"
CLI_DEST="${DEST_DIR}/tsm"

# Utility to check if a URL exists
check_url() {
    local url=$1
    if command -v curl >/dev/null 2>&1; then
        curl -sLfI "$url" >/dev/null 2>&1
    elif command -v wget >/dev/null 2>&1; then
        wget --spider -q "$url" >/dev/null 2>&1
    else
        echo "Error: curl or wget is required."
        exit 1
    fi
}

if [ "$INSTALL_SERVER" = true ]; then
    echo "Checking availability of $SERVER_GITHUB_URL..."
    if ! check_url "$SERVER_GITHUB_URL"; then
        echo "Error: Server binary for ${OS}/${ARCH} is not available on GitHub Releases yet."
        exit 1
    fi
fi

if [ "$INSTALL_CLI" = true ]; then
    echo "Checking availability of $CLI_GITHUB_URL..."
    if ! check_url "$CLI_GITHUB_URL"; then
        echo "Error: CLI binary for ${OS}/${ARCH} is not available on GitHub Releases yet."
        exit 1
    fi
fi

download_file() {
    local url=$1
    local dest=$2
    echo "Downloading from ${url}..."
    if command -v curl >/dev/null 2>&1; then
        curl -sSLf "$url" -o "$dest" || { echo "Failed to download from GitHub."; exit 1; }
    else
        wget -qO "$dest" "$url" || { echo "Failed to download from GitHub."; exit 1; }
    fi
    chmod +x "$dest"
}

if [ "$INSTALL_SERVER" = true ]; then
    echo "Installing Tiny Secrets Manager Server..."
    download_file "$SERVER_GITHUB_URL" "$SERVER_DEST"
    echo "Successfully installed server to ${SERVER_DEST}"
fi

if [ "$INSTALL_CLI" = true ]; then
    echo "Installing Tiny Secrets Manager CLI..."
    download_file "$CLI_GITHUB_URL" "$CLI_DEST"
    echo "Successfully installed CLI to ${CLI_DEST}"
    echo "Run 'tsm --help' to get started!"
fi

if ! echo ":$PATH:" | grep -q ":$DEST_DIR:"; then
    echo ""
    echo "========================================================================"
    echo "WARNING: $DEST_DIR is NOT in your PATH."
    echo "To run the tools from anywhere, add this to your .bashrc or .zshrc:"
    echo "  export PATH=\"\$PATH:$DEST_DIR\""
    echo "========================================================================"
    echo ""
fi
