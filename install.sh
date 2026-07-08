#!/bin/sh
# Copyright (c) 2026 Chatic Contributors
# Licensed under the Apache License, Version 2.0.
#
# One-line installer (Linux/macOS):
#   curl -fsSL https://raw.githubusercontent.com/jousudo/chatic/main/install.sh | sh
#
# Downloads the latest binary from GitHub Releases, installs it and prepares the
# data folder. For auto-start as a service on Linux, prefer the .deb/.rpm package.
#
# FFmpeg (optional, audio only): auto-installed when possible —
#   • macOS: via Homebrew if present (no sudo).
#   • Linux: via the system package manager only when run as root.
#   • Otherwise a hint is printed. Set CHATIC_SKIP_FFMPEG=1 to never touch it.
set -eu

REPO="jousudo/chatic"
PROJECT="chatic"
BIN="chatic"
DATA_DIR="${CHATIC_DATA_DIR:-${TUTOR_DATA_DIR:-$HOME/.chatic}}"

say() { printf '%s\n' "$*"; }
die() { printf '❌ %s\n' "$*" >&2; exit 1; }

# 1. Detect OS and architecture (mapped to the GoReleaser names).
os="$(uname -s)"
arch="$(uname -m)"
case "$os" in
    Linux)  goos="linux" ;;
    Darwin) goos="darwin" ;;
    *) die "OS not supported by this script: $os (on Windows use install.ps1)" ;;
esac
case "$arch" in
    x86_64|amd64) goarch="amd64" ;;
    arm64|aarch64) goarch="arm64" ;;
    *) die "Unsupported architecture: $arch" ;;
esac

# 2. Resolve the version (latest release unless CHATIC_VERSION/TUTOR_VERSION is set).
downloader=""
if command -v curl >/dev/null 2>&1; then downloader="curl"; fi
if [ -z "$downloader" ] && command -v wget >/dev/null 2>&1; then downloader="wget"; fi
[ -n "$downloader" ] || die "You need either 'curl' or 'wget' installed."

fetch() { # fetch <url> <output-file|->
    if [ "$downloader" = "curl" ]; then
        if [ "$2" = "-" ]; then curl -fsSL "$1"; else curl -fsSL -o "$2" "$1"; fi
    else
        if [ "$2" = "-" ]; then wget -qO- "$1"; else wget -qO "$2" "$1"; fi
    fi
}

VERSION="${CHATIC_VERSION:-${TUTOR_VERSION:-}}"
if [ -z "$VERSION" ]; then
    say "🔎 Finding the latest version…"
    VERSION="$(fetch "https://api.github.com/repos/$REPO/releases/latest" - \
        | grep -o '"tag_name" *: *"[^"]*"' | head -1 | cut -d'"' -f4)"
    [ -n "$VERSION" ] || die "Could not find the latest version (has the first release been published yet?)."
fi
ver_num="${VERSION#v}"

# 3. Download and extract the release archive.
archive="${PROJECT}_${ver_num}_${goos}_${goarch}.tar.gz"
url="https://github.com/$REPO/releases/download/$VERSION/$archive"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

say "📥 Downloading $archive ($VERSION)…"
fetch "$url" "$tmp/$archive" || die "Failed to download $url"
tar -xzf "$tmp/$archive" -C "$tmp" || die "Failed to extract the package."
[ -f "$tmp/$BIN" ] || die "Binary '$BIN' not found inside the package."

# 4. Install the binary (system-wide if possible, otherwise into the user's dir).
if [ -w /usr/local/bin ] 2>/dev/null; then
    install_dir="/usr/local/bin"
elif [ "$(id -u)" = "0" ]; then
    install_dir="/usr/local/bin"
else
    install_dir="$HOME/.local/bin"
    mkdir -p "$install_dir"
fi
install -m 0755 "$tmp/$BIN" "$install_dir/$BIN" 2>/dev/null || {
    mkdir -p "$install_dir"; cp "$tmp/$BIN" "$install_dir/$BIN"; chmod 0755 "$install_dir/$BIN";
}

# 5. Prepare the data folder and the .env.
mkdir -p "$DATA_DIR/storage"
if [ ! -f "$DATA_DIR/.env" ] && [ -f "$tmp/.env.example" ]; then
    cp "$tmp/.env.example" "$DATA_DIR/.env"
    chmod 600 "$DATA_DIR/.env" 2>/dev/null || true
fi

# 6. Best-effort FFmpeg (optional; only for audio). Never fatal.
ffmpeg_status="skipped"
ensure_ffmpeg() {
    command -v ffmpeg >/dev/null 2>&1 && { ffmpeg_status="already installed"; return; }
    [ "${CHATIC_SKIP_FFMPEG:-0}" = "1" ] && { ffmpeg_status="skipped (CHATIC_SKIP_FFMPEG=1)"; return; }
    if [ "$goos" = "darwin" ] && command -v brew >/dev/null 2>&1; then
        say "🎧 Installing FFmpeg via Homebrew (optional, for audio)…"
        brew install ffmpeg >/dev/null 2>&1 && ffmpeg_status="installed via brew" || ffmpeg_status="brew install failed — install manually"
        return
    fi
    if [ "$goos" = "linux" ] && [ "$(id -u)" = "0" ]; then
        say "🎧 Installing FFmpeg via the system package manager (optional, for audio)…"
        if command -v apt-get >/dev/null 2>&1; then
            apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq ffmpeg >/dev/null 2>&1 && ffmpeg_status="installed via apt"
        elif command -v dnf >/dev/null 2>&1; then
            dnf install -y -q ffmpeg >/dev/null 2>&1 && ffmpeg_status="installed via dnf"
        elif command -v yum >/dev/null 2>&1; then
            yum install -y -q ffmpeg >/dev/null 2>&1 && ffmpeg_status="installed via yum"
        elif command -v pacman >/dev/null 2>&1; then
            pacman -Sy --noconfirm ffmpeg >/dev/null 2>&1 && ffmpeg_status="installed via pacman"
        elif command -v zypper >/dev/null 2>&1; then
            zypper --non-interactive install ffmpeg >/dev/null 2>&1 && ffmpeg_status="installed via zypper"
        fi
        command -v ffmpeg >/dev/null 2>&1 || ffmpeg_status="not installed — install ffmpeg manually for audio"
        return
    fi
    ffmpeg_status="not installed (optional) — install 'ffmpeg' for voice/TTS"
}
ensure_ffmpeg

say ""
say "✅ Chatic $VERSION installed at: $install_dir/$BIN"
say "------------------------------------------------------------"
case ":$PATH:" in
    *":$install_dir:"*) : ;;
    *) say "⚠️  Add it to your PATH:  export PATH=\"$install_dir:\$PATH\"" ;;
esac
say "▶  To start:"
say "     cd \"$DATA_DIR\" && \"$install_dir/$BIN\""
say "   Then open http://localhost:3030/admin, create the password and scan the QR."
say "🎧 Audio (voice/TTS): FFmpeg $ffmpeg_status."
say "💡 On Linux, to run as a service with auto-start, use the .deb/.rpm package."
