#!/usr/bin/env sh
#
# RAP installer.
#
#   curl -fsSL https://raw.githubusercontent.com/francescobianco/rap/main/install.sh | sh
#
# Downloads the prebuilt binary for this platform from the GitHub releases page
# and installs it. No Go toolchain required.
#
# Environment:
#   RAP_VERSION   release tag to install (default: latest)
#   RAP_BINDIR    install directory (default: $HOME/.local/bin, or /usr/local/bin when root)

set -eu

REPO="francescobianco/rap"
BIN="rap"
VERSION="${RAP_VERSION:-latest}"

if [ -n "${RAP_BINDIR:-}" ]; then
    BINDIR="$RAP_BINDIR"
elif [ "$(id -u)" = "0" ]; then
    BINDIR="/usr/local/bin"
else
    BINDIR="$HOME/.local/bin"
fi

die() { printf 'rap-install: %s\n' "$1" >&2; exit 1; }
info() { printf 'rap-install: %s\n' "$1"; }

need() { command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"; }

detect_platform() {
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    arch=$(uname -m)

    case "$os" in
        linux)   os=linux ;;
        darwin)  os=darwin ;;
        mingw*|msys*|cygwin*) os=windows ;;
        *) die "unsupported operating system: $os" ;;
    esac

    case "$arch" in
        x86_64|amd64)  arch=amd64 ;;
        aarch64|arm64) arch=arm64 ;;
        *) die "unsupported architecture: $arch" ;;
    esac

    printf '%s_%s' "$os" "$arch"
}

download() {
    url="$1"
    out="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$url" -o "$out" || return 1
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$out" "$url" || return 1
    else
        die "need curl or wget to download releases"
    fi
}

need uname
need mkdir

platform=$(detect_platform)
ext=""
case "$platform" in windows_*) ext=".exe" ;; esac

if [ "$VERSION" = "latest" ]; then
    base="https://github.com/$REPO/releases/latest/download"
else
    base="https://github.com/$REPO/releases/download/$VERSION"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

info "installing $BIN ($VERSION) for $platform"

# Release assets are named rap_<os>_<arch> so that the /releases/latest/download
# URL resolves without looking the tag up first. The version lives in the binary.
asset="${BIN}_${platform}${ext}"
if ! download "$base/$asset" "$tmp/$BIN$ext"; then
    die "download failed: $base/$asset"
fi

if download "$base/SHA256SUMS" "$tmp/SHA256SUMS" 2>/dev/null; then
    if command -v sha256sum >/dev/null 2>&1; then
        expected=$(grep " $asset\$" "$tmp/SHA256SUMS" | awk '{print $1}' || true)
        actual=$(sha256sum "$tmp/$BIN$ext" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        expected=$(grep " $asset\$" "$tmp/SHA256SUMS" | awk '{print $1}' || true)
        actual=$(shasum -a 256 "$tmp/$BIN$ext" | awk '{print $1}')
    else
        expected=""
        actual=""
    fi
    if [ -n "$expected" ] && [ "$expected" != "$actual" ]; then
        die "checksum mismatch for $asset"
    fi
    if [ -n "$expected" ]; then info "checksum verified"; fi
fi

chmod +x "$tmp/$BIN$ext"
mkdir -p "$BINDIR"
mv "$tmp/$BIN$ext" "$BINDIR/$BIN$ext"

info "installed $BINDIR/$BIN$ext"
"$BINDIR/$BIN$ext" version || true

case ":$PATH:" in
    *":$BINDIR:"*) ;;
    *) info "note: $BINDIR is not in your PATH; add it with: export PATH=\"$BINDIR:\$PATH\"" ;;
esac
