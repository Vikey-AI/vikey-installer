#!/bin/sh
# Vikey installer for macOS and Linux.
# Downloads the vikey CLI, checks its SHA-256, then starts the setup with a
# one-time code. The code works once and expires after 15 minutes. Your API
# key is never part of this script.
# Source: https://github.com/Vikey-AI/vikey-installer
set -eu

VIKEY_CODE='__CODE__'
VIKEY_VERSION='__VERSION__'
VIKEY_PANEL_URL='__PANEL_URL__'
VIKEY_DOWNLOAD_BASE='__DOWNLOAD_BASE__'

fail() {
  printf '\nVikey: %s\n' "$1" >&2
  exit 1
}

asset_sha() {
  case "$1" in
__ASSET_CASES__
    *) echo "" ;;
  esac
}

main() {
  os=$(uname -s 2>/dev/null || echo unknown)
  arch=$(uname -m 2>/dev/null || echo unknown)
  case "$os" in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    MINGW* | MSYS* | CYGWIN*) fail "Di Windows, buka PowerShell lalu jalankan perintah untuk Windows dari dashboard Vikey." ;;
    *) fail "Sistem operasi $os belum didukung." ;;
  esac
  case "$arch" in
    x86_64 | amd64) arch=amd64 ;;
    arm64 | aarch64) arch=arm64 ;;
    *) fail "Arsitektur $arch belum didukung." ;;
  esac
  # A terminal running under Rosetta on Apple Silicon reports x86_64.
  if [ "$os" = darwin ] && [ "$arch" = amd64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
    arch=arm64
  fi
  target="$os-$arch"
  sha=$(asset_sha "$target")
  [ -n "$sha" ] || fail "Installer untuk $target belum tersedia."

  dir="$HOME/.vikey/bin"
  mkdir -p "$dir" || fail "Tidak bisa membuat folder $dir."
  tmp="$dir/.download.$$"
  trap 'rm -f "$tmp"' EXIT
  trap 'rm -f "$tmp"; exit 130' INT TERM
  url="$VIKEY_DOWNLOAD_BASE/vikey-$target"

  printf 'Mengunduh Vikey CLI %s untuk %s\n' "$VIKEY_VERSION" "$target"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --retry 2 -o "$tmp" "$url" || fail "Gagal mengunduh $url"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O "$tmp" "$url" || fail "Gagal mengunduh $url"
  else
    fail "Butuh curl atau wget untuk mengunduh."
  fi

  if command -v sha256sum >/dev/null 2>&1; then
    got=$(sha256sum "$tmp" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    got=$(shasum -a 256 "$tmp" | awk '{print $1}')
  else
    fail "Butuh sha256sum atau shasum untuk memeriksa file unduhan."
  fi
  [ "$got" = "$sha" ] || fail "File unduhan tidak cocok dengan checksum. Coba lagi beberapa saat lagi."

  chmod 755 "$tmp"
  mv -f "$tmp" "$dir/vikey"
  trap - EXIT INT TERM

  # VIKEY_SETUP_ARGS adds flags, for example "--yes --tools claude,codex" in CI.
  # shellcheck disable=SC2086
  set -- setup --code "$VIKEY_CODE" --panel-url "$VIKEY_PANEL_URL" ${VIKEY_SETUP_ARGS:-}
  # This script arrives through a pipe, so the setup reads its answers from the terminal.
  if (exec </dev/tty) 2>/dev/null; then
    exec "$dir/vikey" "$@" </dev/tty
  fi
  exec "$dir/vikey" "$@"
}

main
