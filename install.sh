#!/bin/sh
# Install emobase from GitHub releases.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/EMOBase/emobase-cli/main/install.sh | sh
#   curl -fsSL .../install.sh | sh -s -- --version 0.2.0
#   curl -fsSL .../install.sh | sh -s -- --user
#
# Options:
#   --version VERSION   Install a specific release (with or without a leading v).
#   --channel CHANNEL   "stable" (default) or "latest" for the rolling main build.
#   --user              Install into ~/.local/bin instead of /usr/local/bin.
#   --prefix DIR        Install into DIR/bin.
#   -h, --help          Show this help.
#
# The archive is always verified against the release's checksums.txt.

set -eu

REPO="EMOBase/emobase-cli"
CHANNEL="stable"
VERSION=""
VERSION_SET=0
USER_INSTALL=0
PREFIX="/usr/local"
PREFIX_SET=0

usage() {
  cat <<'EOF'
Install emobase from GitHub releases.

Usage:
  install.sh [options]

Options:
  --version VERSION   Install a specific release (with or without a leading v).
  --channel CHANNEL   "stable" (default) or "latest" for the rolling main build.
  --user              Install into ~/.local/bin instead of /usr/local/bin.
  --prefix DIR        Install into DIR/bin.
  -h, --help          Show this help.

Examples:
  curl -fsSL https://raw.githubusercontent.com/EMOBase/emobase-cli/main/install.sh | sh
  curl -fsSL .../install.sh | sh -s -- --version 0.2.0
  curl -fsSL .../install.sh | sh -s -- --user
EOF
}

fail() {
  printf 'emobase install: %s\n' "$*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      [ "$#" -ge 2 ] || fail "--version requires a value"
      VERSION="$2"
      VERSION_SET=1
      shift 2
      ;;
    --version=*)
      VERSION="${1#*=}"
      VERSION_SET=1
      shift
      ;;
    --channel)
      [ "$#" -ge 2 ] || fail "--channel requires a value"
      CHANNEL="$2"
      shift 2
      ;;
    --channel=*)
      CHANNEL="${1#*=}"
      shift
      ;;
    --user)
      USER_INSTALL=1
      shift
      ;;
    --prefix)
      [ "$#" -ge 2 ] || fail "--prefix requires a value"
      PREFIX="$2"
      PREFIX_SET=1
      shift 2
      ;;
    --prefix=*)
      PREFIX="${1#*=}"
      PREFIX_SET=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown option: $1 (try --help)"
      ;;
  esac
done

case "$CHANNEL" in
  stable|latest) ;;
  *) fail "--channel must be stable or latest" ;;
esac

if [ "$VERSION_SET" = "1" ]; then
  VERSION="${VERSION#v}"
  [ -n "$VERSION" ] || fail "--version requires a value"
  case "$VERSION" in
    *[!0-9A-Za-z._-]*) fail "invalid version: $VERSION" ;;
  esac
  if [ "$CHANNEL" = "latest" ]; then
    fail "--version and --channel latest cannot be used together"
  fi
fi

if [ "$PREFIX_SET" = "1" ]; then
  [ -n "$PREFIX" ] || fail "--prefix requires a non-empty directory"
fi

if [ "$USER_INSTALL" = "1" ]; then
  [ "$PREFIX_SET" = "0" ] || fail "--user and --prefix cannot be used together"
  [ -n "${HOME:-}" ] || fail "--user requires HOME to be set"
  PREFIX="${HOME}/.local"
fi

# Resolve a relative prefix before the script changes directory to the
# temporary download directory.
case "$PREFIX" in
  /*) ;;
  *) PREFIX="$(pwd)/${PREFIX}" ;;
esac
while [ "${PREFIX%/}" != "$PREFIX" ] && [ "$PREFIX" != "/" ]; do
  PREFIX="${PREFIX%/}"
done

os="$(uname -s)"
case "$os" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) fail "unsupported OS: $os (prebuilt binaries cover Linux and macOS)" ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) fail "unsupported architecture: $arch (prebuilt binaries cover amd64 and arm64)" ;;
esac

need curl
need tar
need install
need awk
need mktemp

if [ "$os" = "linux" ]; then
  need sha256sum
else
  need shasum
fi

if [ -z "$VERSION" ] && [ "$CHANNEL" = "stable" ]; then
  latest_url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
    "https://github.com/${REPO}/releases/latest")"
  VERSION="${latest_url##*/v}"
  [ -n "$VERSION" ] || fail "could not determine the latest stable release"
fi

if [ "$CHANNEL" = "latest" ]; then
  base="https://github.com/${REPO}/releases/download/latest"
  file="emobase_${os}_${arch}.tar.gz"
else
  base="https://github.com/${REPO}/releases/download/v${VERSION}"
  file="emobase_${VERSION}_${os}_${arch}.tar.gz"
fi

tmp="$(mktemp -d "${TMPDIR:-/tmp}/emobase.XXXXXX")"
trap 'rm -rf "$tmp"' 0
cd "$tmp"

printf 'Downloading %s\n' "$file"
curl -fsSLO "$base/$file"
curl -fsSLO "$base/checksums.txt"

awk -v file="$file" '$2 == file { print }' checksums.txt > checksum.txt
[ -s checksum.txt ] || fail "checksum for $file not found in checksums.txt"

if [ "$os" = "linux" ]; then
  sha256sum -c checksum.txt
else
  shasum -a 256 -c checksum.txt
fi

tar -xzf "$file" emobase

bin_dir="${PREFIX%/}/bin"
target="${bin_dir}/emobase"

if mkdir -p "$bin_dir" 2>/dev/null && [ -w "$bin_dir" ]; then
  install -m 0755 emobase "$target"
elif [ "$(id -u)" = "0" ]; then
  mkdir -p "$bin_dir"
  install -m 0755 emobase "$target"
elif command -v sudo >/dev/null 2>&1; then
  printf 'Installing to %s (sudo required)\n' "$target"
  sudo mkdir -p "$bin_dir"
  sudo install -m 0755 emobase "$target"
else
  fail "cannot write $bin_dir; rerun with --user or as root"
fi

printf 'Installed %s\n' "$target"
"$target" --version

case ":$PATH:" in
  *":$bin_dir:"*) ;;
  *) printf '\nAdd %s to PATH to run emobase directly.\n' "$bin_dir" ;;
esac
