#!/bin/bash
# Installation script for Port CLI

set -e

VERSION="${VERSION:-latest}"
REPO="port-labs/port-cli"
GH_API="https://api.github.com/repos/${REPO}"

get_latest_version() {
    if [ "$VERSION" = "latest" ]; then
        curl -s "${GH_API}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/'
    else
        echo "$VERSION"
    fi
}

detect_platform() {
    OS="$(uname -s)"
    ARCH="$(uname -m)"
    
    case "$OS" in
        Linux*)  OS="linux" ;;
        Darwin*) OS="darwin" ;;
        *)       echo "Unsupported OS: $OS" >&2; exit 1 ;;
    esac
    
    case "$ARCH" in
        x86_64) ARCH="amd64" ;;
        arm64|aarch64) ARCH="arm64" ;;
        *)      echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
    esac
    
    echo "${OS}_${ARCH}"
}

install_binary() {
    PLATFORM=$(detect_platform)
    VERSION_TAG=$(get_latest_version)
    BINARY_NAME="port"
    EXT=""
    
    if echo "$PLATFORM" | grep -q "windows"; then
        EXT=".exe"
    fi
    
    VERSION_NO_V=$(echo "$VERSION_TAG" | sed 's/^v//')
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION_TAG}/port-cli_${VERSION_NO_V}_${PLATFORM}${EXT}.tar.gz"
    
    echo "Downloading Port CLI ${VERSION_TAG} for ${PLATFORM}..."
    
    TEMP_DIR=$(mktemp -d)
    trap "rm -rf $TEMP_DIR" EXIT
    
    # Retry download up to 3 times with exponential backoff
    MAX_RETRIES=3
    RETRY_DELAY=2
    DOWNLOAD_SUCCESS=false
    
    # Temporarily disable exit on error for retry logic
    set +e
    for i in $(seq 1 $MAX_RETRIES); do
        if curl -fSL --connect-timeout 10 --max-time 60 "$DOWNLOAD_URL" -o "${TEMP_DIR}/port-cli.tar.gz"; then
            DOWNLOAD_SUCCESS=true
            break
        else
            CURL_EXIT=$?
            if [ $i -eq $MAX_RETRIES ]; then
                echo "" >&2
                echo "Error: Failed to download after $MAX_RETRIES attempts (exit code: $CURL_EXIT)" >&2
                echo "Please check your internet connection and try again." >&2
                echo "You can also manually download from: ${DOWNLOAD_URL}" >&2
                set -e
                exit 1
            fi
            echo "Download failed, retrying in ${RETRY_DELAY} seconds... (attempt $i/$MAX_RETRIES)" >&2
            sleep $RETRY_DELAY
            RETRY_DELAY=$((RETRY_DELAY * 2))
        fi
    done
    set -e
    
    if [ "$DOWNLOAD_SUCCESS" != "true" ]; then
        echo "Error: Download failed" >&2
        exit 1
    fi
    tar -xzf "${TEMP_DIR}/port-cli.tar.gz" -C "$TEMP_DIR"
    
    INSTALL_DIR="${1:-/usr/local/bin}"
    if [ ! -w "$INSTALL_DIR" ]; then
        INSTALL_DIR="$HOME/.local/bin"
        mkdir -p "$INSTALL_DIR"
    fi
    
    mv "${TEMP_DIR}/${BINARY_NAME}${EXT}" "${INSTALL_DIR}/${BINARY_NAME}"
    chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    
    echo "✓ Port CLI installed successfully to ${INSTALL_DIR}/${BINARY_NAME}"
    echo ""
    echo "To get started, run:"
    echo "  ${INSTALL_DIR}/${BINARY_NAME} --help"
    echo ""
    echo "Or configure credentials:"
    echo "  ${INSTALL_DIR}/${BINARY_NAME} config --init"
}

if [ "$1" = "--help" ] || [ "$1" = "-h" ]; then
    echo "Port CLI Installation Script"
    echo ""
    echo "Usage: $0 [VERSION] [INSTALL_DIR]"
    echo ""
    echo "Examples:"
    echo "  $0                    # Install latest version to /usr/local/bin"
    echo "  $0 v1.0.0            # Install specific version"
    echo "  $0 latest ~/.local/bin  # Install to custom directory"
    exit 0
fi

install_binary "$@"

