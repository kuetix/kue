#!/bin/sh
# Kuetix kue CLI — public installer.
#
#   curl -fsSL https://raw.githubusercontent.com/kuetix/kue/main/scripts/install.sh | sh
#
# Downloads the latest (or --version) prebuilt release from
# https://github.com/kuetix/kue/releases, verifies its sha256 against the
# release's checksums.txt, and installs it.
#
# This is the *public* installer for a prebuilt release binary. For a local
# dev build already sitting in runtime/bin/, see ../install.sh instead.
#
# POSIX sh on purpose (piped into `sh`, not guaranteed to be bash).

set -eu

REPO="kuetix/kue"
BINARY="kue"
VERSION=""
INSTALL_DIR="${INSTALL_DIR:-}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { printf "%b\n" "${BLUE}$1${NC}"; }
ok()    { printf "%b\n" "${GREEN}$1${NC}"; }
warn()  { printf "%b\n" "${YELLOW}$1${NC}"; }
error() { printf "%b\n" "${RED}$1${NC}" >&2; }

show_help() {
    cat <<EOF
Kuetix kue CLI — public installer

USAGE:
    install.sh [OPTIONS]

OPTIONS:
    -h, --help          Show this help message
    -d, --dir DIR       Installation directory (default: same resolution as
                         install.sh — \$INSTALL_DIR > \$GOBIN > \$GOPATH/bin >
                         \$HOME/go/bin > /usr/local/bin)
    -v, --version TAG   Install a specific release tag (default: latest),
                         e.g. --version v0.2.0

EXAMPLES:
    curl -fsSL https://raw.githubusercontent.com/kuetix/kue/main/scripts/install.sh | sh
    curl -fsSL .../install.sh | sh -s -- --dir ~/bin
    curl -fsSL .../install.sh | sh -s -- --version v0.2.0
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        -h|--help) show_help; exit 0 ;;
        -d|--dir) INSTALL_DIR="$2"; shift 2 ;;
        -v|--version) VERSION="$2"; shift 2 ;;
        *) error "Unknown option: $1"; show_help; exit 1 ;;
    esac
done

# --- resolve OS / arch -------------------------------------------------

os="$(uname -s)"
case "$os" in
    Linux) GOOS="linux" ;;
    Darwin) GOOS="darwin" ;;
    *)
        error "Unsupported OS: $os"
        warn "Windows: download the .zip directly from https://github.com/${REPO}/releases"
        exit 1
        ;;
esac

arch="$(uname -m)"
case "$arch" in
    x86_64|amd64) GOARCH="amd64" ;;
    aarch64|arm64) GOARCH="arm64" ;;
    *)
        error "Unsupported architecture: $arch"
        exit 1
        ;;
esac

# --- resolve INSTALL_DIR (same order as ../install.sh) -----------------

if [ -z "$INSTALL_DIR" ]; then
    if [ -n "${GOBIN:-}" ]; then
        INSTALL_DIR="$GOBIN"
    elif [ -n "${GOPATH:-}" ]; then
        INSTALL_DIR="$GOPATH/bin"
    elif [ -n "${HOME:-}" ]; then
        INSTALL_DIR="$HOME/go/bin"
    else
        INSTALL_DIR="/usr/local/bin"
    fi
fi

# --- resolve version -----------------------------------------------------

# KUE_INSTALL_API_URL / KUE_INSTALL_BASE_URL let tests point this script at a
# local fake release server instead of the real GitHub endpoints — see
# tests/install/install_test.sh. Real installs never need to set these.
api_url="${KUE_INSTALL_API_URL:-https://api.github.com/repos/${REPO}/releases/latest}"
releases_base_url="${KUE_INSTALL_BASE_URL:-https://github.com/${REPO}/releases/download}"

if [ -z "$VERSION" ]; then
    info "Looking up the latest release..."
    VERSION="$(curl -fsSL "$api_url" \
        | grep '"tag_name"' | head -1 | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
    if [ -z "$VERSION" ]; then
        error "Could not determine the latest release tag from the GitHub API."
        exit 1
    fi
fi
# GoReleaser's {{.Version}} in archive names is the tag without a leading 'v'.
version_num="${VERSION#v}"

archive="kue_${version_num}_${GOOS}_${GOARCH}.tar.gz"
base_url="${releases_base_url}/${VERSION}"

info "Installing kue ${VERSION} (${GOOS}/${GOARCH}) to ${INSTALL_DIR}..."

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

curl -fsSL -o "$tmpdir/$archive" "$base_url/$archive" || {
    error "Failed to download $base_url/$archive"
    error "Check https://github.com/${REPO}/releases for available versions/platforms."
    exit 1
}
curl -fsSL -o "$tmpdir/checksums.txt" "$base_url/checksums.txt" || {
    error "Failed to download checksums.txt for $VERSION"
    exit 1
}

info "Verifying checksum..."
expected="$(grep " $archive\$" "$tmpdir/checksums.txt" | awk '{print $1}')"
if [ -z "$expected" ]; then
    error "No checksum entry for $archive in checksums.txt"
    exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$tmpdir/$archive" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$tmpdir/$archive" | awk '{print $1}')"
else
    error "Neither sha256sum nor shasum found — cannot verify the download."
    exit 1
fi
if [ "$expected" != "$actual" ]; then
    error "Checksum mismatch for $archive"
    error "  expected: $expected"
    error "  actual:   $actual"
    exit 1
fi
ok "Checksum OK"

tar -xzf "$tmpdir/$archive" -C "$tmpdir" "$BINARY"

if [ ! -d "$INSTALL_DIR" ]; then
    warn "Installation directory '$INSTALL_DIR' does not exist — creating it..."
    mkdir -p "$INSTALL_DIR" 2>/dev/null || sudo mkdir -p "$INSTALL_DIR"
fi

target="$INSTALL_DIR/$BINARY"
if cp "$tmpdir/$BINARY" "$target" 2>/dev/null && chmod +x "$target" 2>/dev/null; then
    :
else
    warn "Elevated privileges required for $INSTALL_DIR"
    sudo cp "$tmpdir/$BINARY" "$target"
    sudo chmod +x "$target"
fi

ok "✓ Installed kue ${VERSION} to ${target}"
case ":$PATH:" in
    *":$INSTALL_DIR:"*) : ;;
    *)
        warn "⚠ $INSTALL_DIR is not in your PATH"
        warn "Add it by running: export PATH=\"$INSTALL_DIR:\$PATH\""
        ;;
esac
