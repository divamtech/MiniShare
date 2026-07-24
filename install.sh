#!/bin/sh

# MiniShare CLI installer
# Detects OS/Arch and downloads the latest release from GitHub.

set -e

# Color definitions
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;36m'
NC='\033[0m' # No Color
BOLD='\033[1m'

echo "${BLUE}⚡ MiniShare CLI Installer${NC}"
echo "-----------------------------------"

# Detect OS
OS="$(uname -s)"
case "$OS" in
    Darwin)
        OS_NAME="macOS"
        ;;
    Linux)
        OS_NAME="Linux"
        ;;
    *)
        echo "${RED}Error: Unsupported operating system: $OS${NC}"
        echo "MiniShare precompiled binaries are only supported on macOS and Linux."
        exit 1
        ;;
esac

# GitHub repository details
REPO_OWNER="divamtech"
REPO_NAME="MiniShare"

# Fetch latest version tag from GitHub API
if command -v curl >/dev/null 2>&1; then
    LATEST_TAG=$(curl -s "https://api.github.com/repos/$REPO_OWNER/$REPO_NAME/releases/latest" | grep '"tag_name":' | sed -E 's/.*"tag_name": "([^"]+)".*/\1/')
elif command -v wget >/dev/null 2>&1; then
    LATEST_TAG=$(wget -qO- "https://api.github.com/repos/$REPO_OWNER/$REPO_NAME/releases/latest" | grep '"tag_name":' | sed -E 's/.*"tag_name": "([^"]+)".*/\1/')
fi

if [ -z "$LATEST_TAG" ] || [ "$LATEST_TAG" = "null" ]; then
    echo "${RED}Error: Could not retrieve the latest release version from GitHub API.${NC}"
    echo "This may happen if no releases have been published yet."
    exit 1
fi

echo "Latest Version Found: ${BOLD}$LATEST_TAG${NC}"

# Detect Architecture
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64)
        ARCH_NAME="Intel/AMD64"
        if [ "$OS" = "Darwin" ]; then
            ASSET_NAME="minishare-mac-intel-$LATEST_TAG.zip"
        else
            ASSET_NAME="minishare-linux-$LATEST_TAG.zip"
        fi
        ;;
    arm64|aarch64)
        ARCH_NAME="Apple Silicon/ARM64"
        if [ "$OS" = "Darwin" ]; then
            ASSET_NAME="minishare-mac-silicon-$LATEST_TAG.zip"
        else
            echo "${RED}Error: Precompiled binaries for Linux ARM64 are not yet available.${NC}"
            echo "Please clone the repository and build from source: ${BOLD}go build${NC}"
            exit 1
        fi
        ;;
    *)
        echo "${RED}Error: Unsupported CPU architecture: $ARCH${NC}"
        exit 1
        ;;
esac

echo "Detected: ${BOLD}$OS_NAME ($ARCH_NAME)${NC}"

DOWNLOAD_URL="https://github.com/$REPO_OWNER/$REPO_NAME/releases/download/$LATEST_TAG/$ASSET_NAME"

echo "Downloading ${BOLD}$ASSET_NAME${NC}..."

# Setup temporary directory
TMP_DIR=$(mktemp -d -t minishare-XXXXXX)
cleanup() {
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT

# Download asset
if command -v curl >/dev/null 2>&1; then
    if ! curl -fsSL -o "$TMP_DIR/minishare.zip" "$DOWNLOAD_URL"; then
        echo "${RED}Error: Failed to download from $DOWNLOAD_URL${NC}"
        echo "Please check if a release tag has been created and published on GitHub."
        exit 1
    fi
elif command -v wget >/dev/null 2>&1; then
    if ! wget -q -O "$TMP_DIR/minishare.zip" "$DOWNLOAD_URL"; then
        echo "${RED}Error: Failed to download from $DOWNLOAD_URL${NC}"
        echo "Please check if a release tag has been created and published on GitHub."
        exit 1
    fi
else
    echo "${RED}Error: curl or wget is required to download MiniShare.${NC}"
    exit 1
fi

# Extract
if command -v unzip >/dev/null 2>&1; then
    unzip -q -o "$TMP_DIR/minishare.zip" -d "$TMP_DIR"
else
    echo "${RED}Error: unzip is required to extract the download package.${NC}"
    exit 1
fi

# Check binary exists
if [ ! -f "$TMP_DIR/minishare" ]; then
    echo "${RED}Error: Downloaded package did not contain a 'minishare' binary.${NC}"
    exit 1
fi

# Determine target directory
TARGET_DIR="/usr/local/bin"
if [ ! -d "$TARGET_DIR" ]; then
    echo "Creating target directory $TARGET_DIR..."
    if [ -w "$(dirname $TARGET_DIR)" ]; then
        mkdir -p "$TARGET_DIR"
    else
        sudo mkdir -p "$TARGET_DIR"
    fi
fi

# Install binary
echo "Installing to ${BOLD}$TARGET_DIR/minishare${NC}..."
if [ -w "$TARGET_DIR" ]; then
    cp "$TMP_DIR/minishare" "$TARGET_DIR/minishare"
    chmod +x "$TARGET_DIR/minishare"
else
    echo "${BLUE}Requesting administrator permissions to install to $TARGET_DIR...${NC}"
    sudo cp "$TMP_DIR/minishare" "$TARGET_DIR/minishare"
    sudo chmod +x "$TARGET_DIR/minishare"
fi

echo "-----------------------------------"
echo "${GREEN}✔ MiniShare CLI installed successfully!${NC}"
echo "Run ${BOLD}minishare${NC} in your terminal to start."
