#!/bin/sh
set -eu

OWNER="rfp"
REPO="bkt"
VERSION="${BKT_VERSION:-latest}"
INSTALL_DIR="${BKT_INSTALL_DIR:-}"

log() {
  printf '%s\n' "$*"
}

err() {
  printf 'bkt install: %s\n' "$*" >&2
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    err "missing required command: $1"
    exit 1
  fi
}

need_cmd uname
need_cmd mktemp
need_cmd tar

if command -v curl >/dev/null 2>&1; then
  download() {
    curl -fsSL "$1" -o "$2"
  }
elif command -v wget >/dev/null 2>&1; then
  download() {
    wget -q "$1" -O "$2"
  }
else
  err "missing required command: curl or wget"
  exit 1
fi

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "$os" in
  darwin)
    goos="darwin"
    ;;
  linux)
    goos="linux"
    ;;
  *)
    err "unsupported OS: $os"
    exit 1
    ;;
esac

case "$arch" in
  x86_64|amd64)
    goarch="amd64"
    ;;
  arm64|aarch64)
    goarch="arm64"
    ;;
  *)
    err "unsupported architecture: $arch"
    exit 1
    ;;
esac

if [ "$VERSION" = "latest" ]; then
  release_base="https://github.com/${OWNER}/${REPO}/releases/latest/download"
  display_version="latest"
else
  release_base="https://github.com/${OWNER}/${REPO}/releases/download/${VERSION}"
  display_version="$VERSION"
fi

archive="bkt_${VERSION#v}_${goos}_${goarch}.tar.gz"
if [ "$VERSION" = "latest" ]; then
  err "latest installs require BKT_VERSION for now, for example: BKT_VERSION=v0.1.0 sh install.sh"
  exit 1
fi

if [ -z "$INSTALL_DIR" ]; then
  if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
    INSTALL_DIR="/usr/local/bin"
  else
    INSTALL_DIR="${HOME}/.local/bin"
  fi
fi

mkdir -p "$INSTALL_DIR"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT INT TERM

archive_url="${release_base}/${archive}"
checksums_url="${release_base}/checksums.txt"
archive_path="${tmpdir}/${archive}"
checksums_path="${tmpdir}/checksums.txt"

log "Installing bkt ${display_version} for ${goos}/${goarch}"
log "Downloading ${archive_url}"
download "$archive_url" "$archive_path"

if download "$checksums_url" "$checksums_path"; then
  expected_line="$(grep "  ${archive}$" "$checksums_path" || true)"
  if [ -n "$expected_line" ]; then
    expected_sha="$(printf '%s' "$expected_line" | awk '{print $1}')"
    actual_sha=""

    if command -v sha256sum >/dev/null 2>&1; then
      actual_sha="$(sha256sum "$archive_path" | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
      actual_sha="$(shasum -a 256 "$archive_path" | awk '{print $1}')"
    fi

    if [ -n "$actual_sha" ]; then
      if [ "$actual_sha" != "$expected_sha" ]; then
        err "checksum mismatch for ${archive}"
        err "expected: ${expected_sha}"
        err "actual:   ${actual_sha}"
        exit 1
      fi
      log "Checksum verified"
    else
      log "Checksum file found, but no sha256 tool available; skipping verification"
    fi
  else
    log "Checksum for ${archive} not found; skipping verification"
  fi
else
  log "Could not download checksums.txt; skipping verification"
fi

tar -xzf "$archive_path" -C "$tmpdir"

if [ ! -f "${tmpdir}/bkt" ]; then
  err "archive did not contain bkt binary"
  exit 1
fi

install_path="${INSTALL_DIR}/bkt"
if command -v install >/dev/null 2>&1; then
  install -m 0755 "${tmpdir}/bkt" "$install_path"
else
  cp "${tmpdir}/bkt" "$install_path"
  chmod 0755 "$install_path"
fi

log "Installed bkt to ${install_path}"

if ! command -v bkt >/dev/null 2>&1; then
  log "Note: ${INSTALL_DIR} is not currently on PATH."
  log "Add it to PATH, then run: bkt version"
else
  bkt version || true
fi
