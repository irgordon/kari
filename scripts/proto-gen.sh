#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly MANIFEST="$ROOT_DIR/proto/generated-files.txt"
readonly TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/kari-proto.XXXXXX")"

main() {
  trap cleanup EXIT
  "$ROOT_DIR/scripts/proto-generate-into.sh" "$TEMP_DIR"
  install_generated_outputs
  echo "Protobuf outputs regenerated."
}

install_generated_outputs() {
  local relative_path
  while IFS= read -r relative_path; do
    install -m 0644 "$TEMP_DIR/$relative_path" "$ROOT_DIR/$relative_path"
  done < "$MANIFEST"
}

cleanup() {
  rm -rf "$TEMP_DIR"
}

main "$@"
