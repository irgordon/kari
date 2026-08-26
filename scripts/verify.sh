#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly STAGE_FILE="$ROOT_DIR/scripts/verify-stages.txt"

main() {
  run_verification_contract
}

run_verification_contract() {
  while IFS= read -r stage; do
    run_verification_stage "$stage"
  done < "$STAGE_FILE"
}

run_verification_stage() {
  echo "==> verify-$1"
  make --no-print-directory -C "$ROOT_DIR" "verify-$1"
}

main "$@"
