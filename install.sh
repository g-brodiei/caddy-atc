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
# NOTE: Checksums are verified but not cryptographically signed.
# For maximum security, verify the release artifacts manually.
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

# Install shell completions
CADDY_ATC="${INSTALL_DIR}/caddy-atc"
install_completions() {
    SHELL_NAME=$(basename "${SHELL:-}")

    case "$SHELL_NAME" in
        zsh)
            if [ "$OS" = "darwin" ] && command -v brew >/dev/null 2>&1; then
                COMP_DIR="$(brew --prefix)/share/zsh/site-functions"
            elif [ -d "/usr/local/share/zsh/site-functions" ] && [ -w "/usr/local/share/zsh/site-functions" ]; then
                COMP_DIR="/usr/local/share/zsh/site-functions"
            else
                COMP_DIR="${HOME}/.zsh/completions"
            fi
            COMP_FILE="${COMP_DIR}/_caddy-atc"
            COMP_CMD="zsh"
            ;;
        bash)
            if [ -d "/etc/bash_completion.d" ] && [ -w "/etc/bash_completion.d" ]; then
                COMP_DIR="/etc/bash_completion.d"
            else
                COMP_DIR="${HOME}/.local/share/bash-completion/completions"
            fi
            COMP_FILE="${COMP_DIR}/caddy-atc"
            COMP_CMD="bash"
            ;;
        fish)
            COMP_DIR="${HOME}/.config/fish/completions"
            COMP_FILE="${COMP_DIR}/caddy-atc.fish"
            COMP_CMD="fish"
            ;;
        *)
            echo ""
            echo "Shell completions: run 'caddy-atc completion --help' to set up manually."
            return
            ;;
    esac

    mkdir -p "$COMP_DIR"
    if "$CADDY_ATC" completion "$COMP_CMD" > "$COMP_FILE" 2>/dev/null; then
        echo "Shell completions installed to ${COMP_FILE}"

        # Ensure ~/.zsh/completions is in fpath for zsh user-local installs
        if [ "$COMP_CMD" = "zsh" ] && [ "$COMP_DIR" = "${HOME}/.zsh/completions" ]; then
            ZSHRC="${HOME}/.zshrc"
            if [ -f "$ZSHRC" ] && ! grep -q '\.zsh/completions' "$ZSHRC" 2>/dev/null; then
                printf '\n# caddy-atc completions\nfpath=(~/.zsh/completions $fpath)\nautoload -Uz compinit && compinit\n' >> "$ZSHRC"
                echo "Added ~/.zsh/completions to fpath in ~/.zshrc"
            fi
        fi

        echo "Restart your shell or open a new terminal to activate."
    else
        echo ""
        echo "Shell completions: run 'caddy-atc completion ${COMP_CMD}' to set up manually."
    fi
}

install_completions

echo ""
echo "Run 'caddy-atc --version' to verify."
