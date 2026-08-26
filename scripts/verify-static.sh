#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

main() {
  verify_shell_syntax
  verify_source_identity
  verify_migration_manifest
  reject_floating_dependencies
  verify_stage_matrix
  verify_frontend_contract
  verify_workflows
}

verify_source_identity() (
  local manifest
  manifest="$(mktemp "${TMPDIR:-/tmp}/kari-source-identity.XXXXXX")"
  trap 'rm -f "$manifest"' EXIT
  (cd "$ROOT_DIR" && go test -count=1 ./api/internal/sourceidentity)
  "$ROOT_DIR/scripts/source-identity.sh" --manifest "$manifest" >/dev/null
)

verify_frontend_contract() {
  grep -Fqx '  verify_lints' "$ROOT_DIR/scripts/verify-frontend.sh" || fail "frontend lint stage missing"
  grep -Fqx '  verify_types' "$ROOT_DIR/scripts/verify-frontend.sh" || fail "frontend type-check stage missing"
  grep -Fqx '  verify_production_build' "$ROOT_DIR/scripts/verify-frontend.sh" || fail "frontend build stage missing"
}

verify_stage_matrix() {
  local stage
  while IFS= read -r stage; do
    grep -Fqx "          - $stage" "$ROOT_DIR/.github/workflows/verify.yml" || fail "CI stage missing: $stage"
  done < "$ROOT_DIR/scripts/verify-stages.txt"
}

verify_shell_syntax() {
  local script
  while IFS= read -r script; do
    bash -n "$script"
  done < <(find "$ROOT_DIR/scripts" -maxdepth 1 -type f -name '*.sh' | sort)
}

verify_migration_manifest() {
  (cd "$ROOT_DIR" && go test -count=1 ./api/internal/db -run TestLoadMigrationPlan)
}

reject_floating_dependencies() {
  if rg -n --glob '!verify-static.sh' '(@latest|rust-toolchain@stable|:[[:space:]]*latest)' \
    "$ROOT_DIR/.github" "$ROOT_DIR/scripts" "$ROOT_DIR/api" "$ROOT_DIR/frontend"; then
    fail "floating build dependency found"
  fi
}

verify_workflows() {
  (cd "$ROOT_DIR" && go run ./api/cmd/validate-workflows .)
}

fail() {
  echo "ERROR: $1" >&2
  exit 1
}

main "$@"
