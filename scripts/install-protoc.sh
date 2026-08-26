#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly INSTALL_ROOT="${1:?installation directory is required}"

main() {
  load_versions
  resolve_platform
  download_archive
  verify_archive
  install_archive
}

load_versions() {
  set -a
  source "$ROOT_DIR/toolchain.env"
  set +a
}

resolve_platform() {
  case "$(uname -s)-$(uname -m)" in
    Linux-x86_64)
      ARCHIVE_NAME="protoc-${PROTOC_VERSION}-linux-x86_64.zip"
      ARCHIVE_SHA256="$PROTOC_LINUX_X86_64_SHA256"
      ;;
    Darwin-arm64)
      ARCHIVE_NAME="protoc-${PROTOC_VERSION}-osx-aarch_64.zip"
      ARCHIVE_SHA256="$PROTOC_DARWIN_ARM64_SHA256"
      ;;
    *)
      fail "unsupported protoc platform: $(uname -s)-$(uname -m)"
      ;;
  esac
  readonly ARCHIVE_NAME ARCHIVE_SHA256
  readonly ARCHIVE_PATH="${TMPDIR:-/tmp}/$ARCHIVE_NAME"
}

download_archive() {
  curl --fail --location --silent --show-error \
    --output "$ARCHIVE_PATH" \
    "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/${ARCHIVE_NAME}"
}

verify_archive() {
  local observed
  observed="$(shasum -a 256 "$ARCHIVE_PATH" | awk '{print $1}')"
  [[ "$observed" == "$ARCHIVE_SHA256" ]] || fail "protoc archive checksum mismatch"
}

install_archive() {
  mkdir -p "$INSTALL_ROOT"
  unzip -q -o "$ARCHIVE_PATH" -d "$INSTALL_ROOT"
  "$INSTALL_ROOT/bin/protoc" --version
}

fail() {
  echo "ERROR: $1" >&2
  exit 1
}

main "$@"
