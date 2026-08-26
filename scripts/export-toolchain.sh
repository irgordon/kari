#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

main() {
  load_versions
  export_github_outputs
}

load_versions() {
  set -a
  source "$ROOT_DIR/toolchain.env"
  set +a
}

export_github_outputs() {
  local output_file="${GITHUB_OUTPUT:?GITHUB_OUTPUT is required}"
  printf 'go=%s\n' "$GO_VERSION" >> "$output_file"
  printf 'rust=%s\n' "$RUST_VERSION" >> "$output_file"
  printf 'node=%s\n' "$NODE_VERSION" >> "$output_file"
  printf 'npm=%s\n' "$NPM_VERSION" >> "$output_file"
}

main "$@"
