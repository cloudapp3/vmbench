#!/usr/bin/env bash
set -euo pipefail

REPO="cloudapp3/vmbench"
BINARY_NAME="vmbench"
INSTALL_DIR="${VMBENCH_INSTALL_DIR:-}"
VERSION=""
SKIP_VERIFY=0
PRINT_INSTALL_DIR=0
ARCHIVE_URL_OVERRIDE="${VMBENCH_ARCHIVE_URL:-}"
CHECKSUMS_URL_OVERRIDE="${VMBENCH_CHECKSUMS_URL:-}"
DOWNLOAD_HEADER_1="${VMBENCH_DOWNLOAD_HEADER_1:-}"
DOWNLOAD_HEADER_2="${VMBENCH_DOWNLOAD_HEADER_2:-}"
CUSTOM_RELEASE_SOURCE=0

usage() {
  cat <<'EOF'
Install vmbench from GitHub Releases.

Usage:
  install.sh [--version vX.Y.Z] [--dir PATH] [--skip-verify]
             [--print-install-dir]

Options:
  --version <tag>   Install a specific release tag. Defaults to the latest release.
  --dir <path>      Install directory. Auto-detected in order: /usr/local/bin,
                    ~/.local/bin, ~/bin. Override with VMBENCH_INSTALL_DIR env var.
  --skip-verify     Skip SHA-256 checksum verification.
  --print-install-dir
                    Print the selected install directory to stdout after success.
  -h, --help        Show this help message.

Examples:
  curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | bash
  curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | sudo bash
  curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | bash -s -- --version v0.1.0
EOF
}

log() {
  printf '%s\n' "$*" >&2
}

die() {
  log "Error: $*"
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      shift
      [ "$#" -gt 0 ] || die "missing value for --version"
      VERSION="$1"
      ;;
    --version=*)
      VERSION="${1#*=}"
      ;;
    --dir)
      shift
      [ "$#" -gt 0 ] || die "missing value for --dir"
      INSTALL_DIR="$1"
      ;;
    --dir=*)
      INSTALL_DIR="${1#*=}"
      ;;
    --skip-verify)
      SKIP_VERIFY=1
      ;;
    --print-install-dir)
      PRINT_INSTALL_DIR=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
  shift
done

if [ -n "$ARCHIVE_URL_OVERRIDE" ] || [ -n "$CHECKSUMS_URL_OVERRIDE" ]; then
  [ -n "$ARCHIVE_URL_OVERRIDE" ] && [ -n "$CHECKSUMS_URL_OVERRIDE" ] \
    || die "VMBENCH_ARCHIVE_URL and VMBENCH_CHECKSUMS_URL must be set together"
  [ -n "$VERSION" ] \
    || die "--version is required with a custom release source"
  for release_url in "$ARCHIVE_URL_OVERRIDE" "$CHECKSUMS_URL_OVERRIDE"; do
    case "$release_url" in
      http://*|https://*) ;;
      *) die "custom release URLs must start with http:// or https://" ;;
    esac
  done
  CUSTOM_RELEASE_SOURCE=1
elif [ -n "$DOWNLOAD_HEADER_1" ] || [ -n "$DOWNLOAD_HEADER_2" ]; then
  die "custom download headers require VMBENCH_ARCHIVE_URL and VMBENCH_CHECKSUMS_URL"
fi

for download_header in "$DOWNLOAD_HEADER_1" "$DOWNLOAD_HEADER_2"; do
  [ -z "$download_header" ] && continue
  case "$download_header" in
    *$'\r'*|*$'\n'*) die "custom download headers must be single-line values" ;;
    *:*) ;;
    *) die "custom download headers must use 'Name: value' syntax" ;;
  esac
done

require_cmd curl
require_cmd tar
require_cmd uname
require_cmd mktemp

if [ -z "$INSTALL_DIR" ]; then
  for d in /usr/local/bin "$HOME/.local/bin" "$HOME/bin"; do
    if [ -w "$d" ] 2>/dev/null || [ -w "$(dirname "$d")" ] 2>/dev/null; then
      INSTALL_DIR="$d"
      break
    fi
  done
  INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
fi

case "$(uname -s)" in
  Linux) OS="linux" ;;
  Darwin) OS="darwin" ;;
  *)
    die "unsupported operating system: $(uname -s)"
    ;;
esac

case "$(uname -m)" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)
    die "unsupported architecture: $(uname -m)"
    ;;
esac

TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
CURL_ARGS=(-fsSL)
if [ -n "${TOKEN}" ]; then
  CURL_ARGS+=(
    -H "Authorization: Bearer ${TOKEN}"
    -H "Accept: application/vnd.github+json"
    -H "X-GitHub-Api-Version: 2022-11-28"
  )
fi
DOWNLOAD_CURL_ARGS=("${CURL_ARGS[@]}")
if [ "$CUSTOM_RELEASE_SOURCE" -eq 1 ]; then
  # Do not forward GitHub credentials to an explicitly configured host.
  DOWNLOAD_CURL_ARGS=(-fsSL)
  [ -z "$DOWNLOAD_HEADER_1" ] || DOWNLOAD_CURL_ARGS+=(-H "$DOWNLOAD_HEADER_1")
  [ -z "$DOWNLOAD_HEADER_2" ] || DOWNLOAD_CURL_ARGS+=(-H "$DOWNLOAD_HEADER_2")
fi

curl_text() {
  curl "${CURL_ARGS[@]}" "$1"
}

curl_download() {
  local url="$1"
  local output="$2"
  curl "${DOWNLOAD_CURL_ARGS[@]}" -o "$output" "$url"
}

if [ -z "$VERSION" ]; then
  log "Resolving latest release from ${REPO}..."
  VERSION="$(
    curl_text "https://api.github.com/repos/${REPO}/releases/latest" \
      | sed -n 's/^[[:space:]]*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' \
      | head -n 1
  )"
  [ -n "$VERSION" ] || die "failed to resolve latest release tag"
fi

case "$VERSION" in
  v*) ;;
  *) VERSION="v${VERSION}" ;;
esac

VERSION_NO_V="${VERSION#v}"
ARCHIVE_NAME="${BINARY_NAME}-${VERSION_NO_V}-${OS}-${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
if [ "$CUSTOM_RELEASE_SOURCE" -eq 1 ]; then
  ARCHIVE_URL="$ARCHIVE_URL_OVERRIDE"
  CHECKSUMS_URL="$CHECKSUMS_URL_OVERRIDE"
else
  ARCHIVE_URL="${BASE_URL}/${ARCHIVE_NAME}"
  CHECKSUMS_URL="${BASE_URL}/checksums.txt"
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ARCHIVE_PATH="${TMPDIR}/${ARCHIVE_NAME}"
CHECKSUMS_PATH="${TMPDIR}/checksums.txt"

log "Downloading ${ARCHIVE_NAME}..."
curl_download "$ARCHIVE_URL" "$ARCHIVE_PATH" || die "failed to download ${ARCHIVE_URL}"

if [ "$SKIP_VERIFY" -eq 0 ]; then
  log "Downloading checksums.txt..."
  curl_download "$CHECKSUMS_URL" "$CHECKSUMS_PATH" || die "failed to download ${CHECKSUMS_URL}"

  EXPECTED_SUM="$(
    awk -v name="$ARCHIVE_NAME" '$2 == name { print $1 }' "$CHECKSUMS_PATH"
  )"
  [ -n "$EXPECTED_SUM" ] || die "checksum entry not found for ${ARCHIVE_NAME}"

  if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL_SUM="$(sha256sum "$ARCHIVE_PATH" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    ACTUAL_SUM="$(shasum -a 256 "$ARCHIVE_PATH" | awk '{print $1}')"
  else
    die "checksum verification requested but neither sha256sum nor shasum is available"
  fi

  [ "$ACTUAL_SUM" = "$EXPECTED_SUM" ] || die "checksum mismatch for ${ARCHIVE_NAME}"
  log "Checksum verified."
else
  log "Skipping checksum verification."
fi

log "Extracting archive..."
tar -xzf "$ARCHIVE_PATH" -C "$TMPDIR"

BINARY_PATH="${TMPDIR}/${BINARY_NAME}"
if [ ! -f "$BINARY_PATH" ]; then
  BINARY_PATH="$(find "$TMPDIR" -maxdepth 2 -type f -name "$BINARY_NAME" | head -n 1 || true)"
fi
[ -n "${BINARY_PATH}" ] && [ -f "$BINARY_PATH" ] || die "failed to find ${BINARY_NAME} in archive"

if [ ! -d "$INSTALL_DIR" ]; then
  mkdir -p "$INSTALL_DIR" 2>/dev/null || die "cannot create install directory: ${INSTALL_DIR}"
fi
[ -w "$INSTALL_DIR" ] || die "install directory is not writable: ${INSTALL_DIR}"

TARGET_PATH="${INSTALL_DIR}/${BINARY_NAME}"
if command -v install >/dev/null 2>&1; then
  install -m 0755 "$BINARY_PATH" "$TARGET_PATH"
else
  cp "$BINARY_PATH" "$TARGET_PATH"
  chmod 0755 "$TARGET_PATH"
fi

log "Installed ${BINARY_NAME} ${VERSION} to ${TARGET_PATH}"
log "Verify the installation with:"
log "  ${TARGET_PATH} version"
if [ "$PRINT_INSTALL_DIR" -eq 1 ]; then
  printf '%s\n' "$INSTALL_DIR"
fi
