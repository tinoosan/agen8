#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TOOLS_DIR="${AGEN8_TOOLS_DIR:-$ROOT_DIR/.tools}"
BIN_DIR="$TOOLS_DIR/bin"
SRC_DIR="$TOOLS_DIR/src"

AIR_VERSION="${AIR_VERSION:-v1.63.1}"
NODE_VERSION="${NODE_VERSION:-22.12.0}"

mkdir -p "$BIN_DIR" "$SRC_DIR"

log() {
  printf '[bootstrap] %s\n' "$*" >&2
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

cpu_jobs() {
  getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4
}

ensure_air() {
  if command -v air >/dev/null 2>&1; then
    command -v air
    return
  fi

  local air_bin="$BIN_DIR/air"
  if [ -x "$air_bin" ]; then
    printf '%s\n' "$air_bin"
    return
  fi

  require_cmd go
  log "installing air $AIR_VERSION from source via go install"
  GOBIN="$BIN_DIR" go install "github.com/air-verse/air@$AIR_VERSION"
  printf '%s\n' "$air_bin"
}

ensure_npm() {
  if command -v npm >/dev/null 2>&1; then
    command -v npm
    return
  fi

  local prefix="$TOOLS_DIR/node-v$NODE_VERSION"
  local npm_bin="$prefix/bin/npm"
  if [ -x "$npm_bin" ]; then
    printf '%s\n' "$npm_bin"
    return
  fi

  require_cmd curl
  require_cmd tar
  require_cmd make
  require_cmd python3

  local cc_cmd="${CC:-cc}"
  if ! command -v "$cc_cmd" >/dev/null 2>&1; then
    printf 'missing required C compiler: %s\n' "$cc_cmd" >&2
    exit 1
  fi

  local archive="$SRC_DIR/node-v$NODE_VERSION.tar.gz"
  local extract_dir="$SRC_DIR/node-v$NODE_VERSION"
  local url="https://nodejs.org/dist/v$NODE_VERSION/node-v$NODE_VERSION.tar.gz"

  if [ ! -f "$archive" ]; then
    log "downloading node v$NODE_VERSION source"
    curl -fsSL "$url" -o "$archive"
  fi

  rm -rf "$extract_dir"
  tar -xzf "$archive" -C "$SRC_DIR"

  log "building node v$NODE_VERSION from source into $prefix"
  (
    cd "$extract_dir"
    ./configure --prefix="$prefix"
    make -j"$(cpu_jobs)"
    make install
  )

  printf '%s\n' "$npm_bin"
}

case "${1:-}" in
  air)
    ensure_air
    ;;
  npm)
    ensure_npm
    ;;
  *)
    printf 'usage: %s <air|npm>\n' "$0" >&2
    exit 1
    ;;
esac
