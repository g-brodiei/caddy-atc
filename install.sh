#!/bin/sh
# Install script for caddy-atc
# Usage: curl -fsSL https://raw.githubusercontent.com/g-brodiei/caddy-atc/main/install.sh | sh
# Override version: VERSION=0.1.0 curl -fsSL ... | sh
# Override install dir: INSTALL_DIR=/usr/local/bin curl -fsSL ... | sh

set -eu

REPO="g-brodiei/caddy-atc"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

cleanup() {
    rm -rf "$TMPDIR"
}

fail() {
    echo "Error: $1" >&2
    exit 1
}

# Detect OS
detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        *)       fail "Unsupported OS: $(uname -s). caddy-atc supports Linux and macOS." ;;
    esac
}

# Detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)             fail "Unsupported architecture: $(uname -m). caddy-atc supports amd64 and arm64." ;;
    esac
}

# Get latest version from GitHub API (no jq dependency)
get_latest_version() {
    url="https://api.github.com/repos/${REPO}/releases/latest"
    tag=$(curl -fsSL "$url" | grep '"tag_name"' | sed -E 's/.*"tag_name":\s*"([^"]+)".*/\1/')
    if [ -z "$tag" ]; then
        fail "Could not determine latest version. Check https://github.com/${REPO}/releases"
    fi
    # Strip leading v
    echo "$tag" | sed 's/^v//'
}

OS=$(detect_os)
ARCH=$(detect_arch)

echo "Detected: ${OS}/${ARCH}"

# Determine version
if [ -n "${VERSION:-}" ]; then
    # Strip leading v if user provided it
    VERSION=$(echo "$VERSION" | sed 's/^v//')
    echo "Installing caddy-atc v${VERSION} (pinned)"
else
    echo "Fetching latest version..."
    VERSION=$(get_latest_version)
    echo "Latest version: v${VERSION}"
fi

# Set up temp directory
TMPDIR=$(mktemp -d)
trap cleanup EXIT

# Download archive and checksums
ARCHIVE="caddy-atc_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/v${VERSION}"

echo "Downloading ${ARCHIVE}..."
curl -fsSL -o "${TMPDIR}/${ARCHIVE}" "${BASE_URL}/${ARCHIVE}" || fail "Download failed. Check that v${VERSION} exists at https://github.com/${REPO}/releases"

echo "Downloading checksums..."
curl -fsSL -o "${TMPDIR}/checksums.txt" "${BASE_URL}/checksums.txt" || fail "Checksum download failed."

# Verify checksum
echo "Verifying checksum..."
cd "$TMPDIR"
expected=$(grep "${ARCHIVE}" checksums.txt | awk '{print $1}')
if [ -z "$expected" ]; then
    fail "Archive not found in checksums.txt"
fi

if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "${ARCHIVE}" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "${ARCHIVE}" | awk '{print $1}')
else
    fail "No sha256sum or shasum found. Cannot verify checksum."
fi

if [ "$expected" != "$actual" ]; then
    fail "Checksum mismatch!\n  Expected: ${expected}\n  Actual:   ${actual}"
fi
echo "Checksum verified."

# Extract
tar xzf "${ARCHIVE}"

# Install
echo "Installing to ${INSTALL_DIR}/caddy-atc..."
if [ -w "$INSTALL_DIR" ]; then
    mv caddy-atc "${INSTALL_DIR}/caddy-atc"
else
    sudo mv caddy-atc "${INSTALL_DIR}/caddy-atc"
fi

echo "caddy-atc v${VERSION} installed successfully!"
echo ""
echo "Run 'caddy-atc --version' to verify."
