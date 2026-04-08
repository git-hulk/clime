#!/bin/sh
set -eu

# clime installer
# Usage:
#   curl -sSfL https://raw.githubusercontent.com/git-hulk/clime/master/scripts/install.sh | sh
#   curl -sSfL ... | sh -s -- --plugin hr
#   curl -sSfL ... | sh -s -- --dir /usr/local/bin

OWNER="git-hulk"
BINARY="clime"
REPO="${OWNER}/${BINARY}"
INSTALL_DIR="${CLIME_INSTALL_DIR:-}"
TARGET_OS="${CLIME_OS:-}"
TARGET_ARCH="${CLIME_ARCH:-}"
PLUGIN=""

# Parse arguments
while [ $# -gt 0 ]; do
    case "$1" in
        --plugin)
            PLUGIN="$2"
            shift 2
            ;;
        --dir)
            INSTALL_DIR="$2"
            shift 2
            ;;
        *)
            shift
            ;;
    esac
done

# If installing a plugin, adjust repo and binary name
if [ -n "$PLUGIN" ]; then
    BINARY="clime-${PLUGIN}"
    REPO="${OWNER}/${BINARY}"
fi

# Default install directory
if [ -z "$INSTALL_DIR" ]; then
    if [ -n "$PLUGIN" ]; then
        INSTALL_DIR="${HOME}/.clime/plugins"
    else
        INSTALL_DIR="${HOME}/.local/bin"
    fi
fi

# Detect OS
if [ -n "$TARGET_OS" ]; then
    OS=$(printf '%s' "$TARGET_OS" | tr '[:upper:]' '[:lower:]')
else
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
fi
case "$OS" in
    darwin|linux) ;;
    *)
        echo "Error: unsupported OS: $OS"
        exit 1
        ;;
esac

# Detect architecture
if [ -n "$TARGET_ARCH" ]; then
    ARCH=$(printf '%s' "$TARGET_ARCH" | tr '[:upper:]' '[:lower:]')
else
    ARCH=$(uname -m | tr '[:upper:]' '[:lower:]')
fi
case "$ARCH" in
    x86_64|x64|amd64)      ARCH="amd64" ;;
    aarch64|arm64|armv8l)  ARCH="arm64" ;;
    *)
        echo "Error: unsupported architecture: $ARCH"
        exit 1
        ;;
esac

echo "Detected platform: ${OS}/${ARCH}"

# Fetch latest release tag
echo "Fetching latest release..."
GITHUB_API_URL="https://api.github.com/repos/${REPO}/releases/latest"

AUTH_HEADER=""
if [ -n "${GITHUB_TOKEN:-}" ]; then
    AUTH_HEADER="Authorization: Bearer ${GITHUB_TOKEN}"
fi

if [ -n "$AUTH_HEADER" ]; then
    RELEASE_TAG=$(curl -sSf -H "$AUTH_HEADER" "$GITHUB_API_URL" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//')
else
    # Try the API first; fall back to the redirect-based approach if rate-limited
    RELEASE_TAG=$(curl -sSf "$GITHUB_API_URL" 2>/dev/null | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//' || true)
    if [ -z "$RELEASE_TAG" ]; then
        RELEASE_TAG=$(curl -sSfI "https://github.com/${REPO}/releases/latest" 2>/dev/null | grep -i '^location:' | sed 's|.*/tag/||;s/[[:space:]]*$//')
    fi
fi

if [ -z "$RELEASE_TAG" ]; then
    echo "Error: could not determine latest release for ${REPO}"
    exit 1
fi

VERSION="${RELEASE_TAG#v}"
echo "Latest version: ${VERSION}"

# Download
ARCHIVE_NAME="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${RELEASE_TAG}/${ARCHIVE_NAME}"

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading ${ARCHIVE_NAME}..."
if [ -n "$AUTH_HEADER" ]; then
    curl -sSfL -H "$AUTH_HEADER" -o "${TMPDIR}/${ARCHIVE_NAME}" "$DOWNLOAD_URL"
else
    curl -sSfL -o "${TMPDIR}/${ARCHIVE_NAME}" "$DOWNLOAD_URL"
fi

# Extract
echo "Extracting..."
tar -xzf "${TMPDIR}/${ARCHIVE_NAME}" -C "$TMPDIR"

# Install
mkdir -p "$INSTALL_DIR"
mv "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
chmod +x "${INSTALL_DIR}/${BINARY}"

echo "Installed ${BINARY} to ${INSTALL_DIR}/${BINARY}"

# Add to PATH if needed (only for the main CLI, not plugins)
if [ -z "$PLUGIN" ]; then
    case ":${PATH}:" in
        *":${INSTALL_DIR}:"*)
            ;;
        *)
            SHELL_RC=""
            case "${SHELL:-}" in
                */zsh)  SHELL_RC="$HOME/.zshrc" ;;
                */bash) SHELL_RC="$HOME/.bashrc" ;;
            esac
            if [ -n "$SHELL_RC" ] && [ -f "$SHELL_RC" ]; then
                if ! grep -q "$INSTALL_DIR" "$SHELL_RC" 2>/dev/null; then
                    echo "" >> "$SHELL_RC"
                    echo "# clime" >> "$SHELL_RC"
                    echo "export PATH=\"${INSTALL_DIR}:\$PATH\"" >> "$SHELL_RC"
                    echo "Added ${INSTALL_DIR} to PATH in ${SHELL_RC}"
                    echo "Run 'source ${SHELL_RC}' or restart your shell to use it."
                fi
            else
                echo ""
                echo "Add the following to your shell profile:"
                echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
            fi
            ;;
    esac
fi

# Run init to install default plugins (only for the main CLI, not plugin installs)
if [ -z "$PLUGIN" ]; then
    echo ""
    PATH="${INSTALL_DIR}:${PATH}" "${INSTALL_DIR}/${BINARY}" init || true
fi

echo ""
echo "Done! Run '${BINARY} version' to verify."
