#!/bin/bash
set -e

SERVER_URL=""
DEST_DIR="/usr/local/bin"

while [[ "$#" -gt 0 ]]; do
    case $1 in
        --dest) DEST_DIR="$2"; shift ;;
        *) SERVER_URL="$1" ;;
    esac
    shift
done

if [ -z "$SERVER_URL" ]; then
    echo "Error: Server URL is required."
    echo "Usage: curl -sSL <server-url>/install.sh | bash -s <server-url> [--dest <path>]"
    echo "Example: curl -sSL http://localhost:8090/install.sh | bash -s http://localhost:8090"
    exit 1
fi

echo "Ensuring directory $DEST_DIR exists and is writable..."
if ! mkdir -p "$DEST_DIR" 2>/dev/null || ! touch "$DEST_DIR/.tsm_test" 2>/dev/null; then
    echo "Error: You do not have write permissions for $DEST_DIR."
    echo "Please specify a different destination directory you have access to, or run the script as a user with appropriate permissions."
    echo "Example (custom directory): curl -sSL $SERVER_URL/install.sh | bash -s $SERVER_URL --dest ~/.local/bin"
    echo "Example (with sudo):        curl -sSL $SERVER_URL/install.sh | sudo bash -s $SERVER_URL"
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

BINARY_NAME="tsm-${OS}-${ARCH}"
DOWNLOAD_URL="${SERVER_URL}/cli/${BINARY_NAME}"
DEST="${DEST_DIR}/tsm"

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

echo "Checking availability of $DOWNLOAD_URL..."
if ! check_url "$DOWNLOAD_URL"; then
    echo "Error: CLI binary for ${OS}/${ARCH} is not available on the Server."
    exit 1
fi

echo "Downloading ${BINARY_NAME} from ${DOWNLOAD_URL}..."

if command -v curl >/dev/null 2>&1; then
    curl -sSLf "$DOWNLOAD_URL" -o "$DEST" || { echo "Failed to download CLI from the Server."; exit 1; }
else
    wget -qO "$DEST" "$DOWNLOAD_URL" || { echo "Failed to download CLI from the Server."; exit 1; }
fi

chmod +x "$DEST"
echo "Successfully installed tsm CLI to ${DEST}"
echo "Run 'tsm --help' to get started!"

if ! echo ":$PATH:" | grep -q ":$DEST_DIR:"; then
    echo ""
    echo "========================================================================"
    echo "WARNING: $DEST_DIR is NOT in your PATH."
    echo "To run the tools from anywhere, add this to your .bashrc or .zshrc:"
    echo "  export PATH=\"\$PATH:$DEST_DIR\""
    echo "========================================================================"
    echo ""
fi
