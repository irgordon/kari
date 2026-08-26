#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly MANIFEST="$ROOT_DIR/proto/generated-files.txt"
readonly TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/kari-proto-check.XXXXXX")"

main() {
  trap cleanup EXIT
  "$ROOT_DIR/scripts/proto-generate-into.sh" "$TEMP_DIR"
  verify_manifest_matches_repository
  compare_generated_outputs
  echo "Protobuf outputs match the pinned generators."
}

verify_manifest_matches_repository() {
  local actual_manifest
  actual_manifest="$(find_expected_outputs)"
  diff -u "$MANIFEST" <(printf '%s\n' "$actual_manifest") || fail "generated-file manifest drift"
}

find_expected_outputs() {
  {
    find "$ROOT_DIR/api/internal/grpc/rustagent" -maxdepth 1 -type f -name '*.pb.go'
    find "$ROOT_DIR/agent/src/proto" -maxdepth 1 -type f -name 'agent_descriptor.bin'
  } | sed "s#^$ROOT_DIR/##" | sort
}

compare_generated_outputs() {
  local relative_path
  local drift=0
  while IFS= read -r relative_path; do
    compare_output "$relative_path" || drift=1
  done < "$MANIFEST"
  [[ "$drift" -eq 0 ]] || fail "protobuf drift detected; run 'make proto'"
}

compare_output() {
  local relative_path="$1"
  if cmp -s "$ROOT_DIR/$relative_path" "$TEMP_DIR/$relative_path"; then
    return 0
  fi
  echo "DRIFT: $relative_path" >&2
  if [[ "$relative_path" == *.go ]]; then
    diff -u "$ROOT_DIR/$relative_path" "$TEMP_DIR/$relative_path" || true
  fi
  return 1
}

cleanup() {
  rm -rf "$TEMP_DIR"
}

fail() {
  echo "ERROR: $1" >&2
  exit 1
}

main "$@"
