#!/usr/bin/env bash

set -euo pipefail

VERSION="${CASOS_VERSION:-latest}"
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
INSTALL_DIR="${INSTALL_DIR:-$BIN_DIR}"
REPO="casosorg/casos"

info() { printf '%s\n' "$*"; }
die() { printf '[casos] %s\n' "$*" >&2; exit 1; }

command -v curl >/dev/null 2>&1 || die "required command not found: curl"

if [[ "$VERSION" == "latest" ]]; then
  info "Fetching latest CasOS release..."
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
    sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  [[ -n "$VERSION" ]] || die "could not resolve the latest release"
fi

[[ "$VERSION" =~ ^v[0-9A-Za-z._-]+$ ]] || die "invalid release version: $VERSION"

case "$(uname -s)" in
  Linux) os_name="linux" ;;
  *) die "unsupported operating system: $(uname -s)" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch_name="amd64" ;;
  arm64|aarch64) arch_name="arm64" ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

filename="casos_${os_name}_${arch_name}"
release_url="https://github.com/$REPO/releases/download/$VERSION"
temp_dir="$(mktemp -d)"
trap 'rm -rf "$temp_dir"' EXIT

info "Downloading CasOS $VERSION..."
curl -fsSL --retry 3 -o "$temp_dir/$filename" "$release_url/$filename"
curl -fsSL --retry 3 -o "$temp_dir/SHA256SUMS" "$release_url/SHA256SUMS"

expected="$(sed -n "s/^\([[:xdigit:]]\{64\}\)[[:space:]]\+\*\{0,1\}${filename}$/\1/p" "$temp_dir/SHA256SUMS")"
[[ -n "$expected" ]] || die "release checksum for $filename was not found"
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$temp_dir/$filename" | cut -d ' ' -f 1)"
elif command -v shasum >/dev/null 2>&1; then
  actual="$(shasum -a 256 "$temp_dir/$filename" | cut -d ' ' -f 1)"
else
  die "sha256sum or shasum is required to verify the download"
fi
[[ "$actual" == "$expected" ]] || die "download checksum verification failed"

mkdir -p "$INSTALL_DIR" "$BIN_DIR"
INSTALL_DIR="$(cd "$INSTALL_DIR" && pwd -P)"
BIN_DIR="$(cd "$BIN_DIR" && pwd -P)"
install -m 0755 "$temp_dir/$filename" "$INSTALL_DIR/casos.new"
mv -f "$INSTALL_DIR/casos.new" "$INSTALL_DIR/casos"
if [[ "$BIN_DIR/casos" != "$INSTALL_DIR/casos" ]]; then
  ln -sfn "$INSTALL_DIR/casos" "$BIN_DIR/casos"
fi

if [[ ":$PATH:" != *":$BIN_DIR:"* ]]; then
  info "$BIN_DIR is not on PATH; add it to your shell profile to run 'casos' later."
fi

info "CasOS $VERSION installed at $INSTALL_DIR/casos"
info "Starting CasOS at http://127.0.0.1:9000 ..."
export PATH="$BIN_DIR:$PATH"
exec "$INSTALL_DIR/casos"
