#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly TEMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/kari-negative.XXXXXX")"
readonly PRISTINE_DIR="$TEMP_ROOT/pristine"
readonly RESULTS_DIR="$ROOT_DIR/.artifacts/negative-controls"
readonly RESULTS_FILE="$ROOT_DIR/.artifacts/negative-controls.tsv"
SOURCE_IDENTITY=""

main() {
  trap cleanup EXIT
  SOURCE_IDENTITY="$(source_identity)"
  readonly SOURCE_IDENTITY
  prepare_artifacts
  prepare_pristine
  run_original_controls
  run_toolchain_controls
  assert_source_unchanged
  echo "Negative controls passed; results: $RESULTS_FILE"
}

run_original_controls() {
  control_generated_output
  control_proto_source
  control_repository_column
  control_missing_migration
  control_migration_order
  control_toolchain_conflict
  control_missing_dockerfile
  control_health_endpoint
  control_frontend_stage
  control_documented_path
}

run_toolchain_controls() {
  control_stale_go
  control_stale_node
  control_stale_npm
  control_stale_rust
  control_mutable_action
  control_unused_action_policy
}

prepare_artifacts() {
  mkdir -p "$RESULTS_DIR"
  printf 'control\tmutation\texpected_stage\tobserved_failure\texit\trestored_identity\tcommand\n' > "$RESULTS_FILE"
}

prepare_pristine() {
  mkdir -p "$PRISTINE_DIR"
  (cd "$ROOT_DIR" && tar \
    --exclude='./.git' --exclude='./.artifacts' --exclude='./.runtime' \
    --exclude='./frontend/node_modules' --exclude='./frontend/dist' --exclude='./agent/target' \
    -cf - .) | (cd "$PRISTINE_DIR" && tar -xf -)
  git -C "$PRISTINE_DIR" init --quiet
  git -C "$PRISTINE_DIR" config user.email negative-controls@example.test
  git -C "$PRISTINE_DIR" config user.name "Kari Negative Controls"
  git -C "$PRISTINE_DIR" config commit.gpgsign false
  git -C "$PRISTINE_DIR" add -A
  git -C "$PRISTINE_DIR" commit --quiet -m pristine
}

control_generated_output() {
  local case_dir
  case_dir="$(new_case proto-output)"
  printf '\n// negative control\n' >> "$case_dir/api/internal/grpc/rustagent/agent.pb.go"
  expect_failure "$case_dir" proto-output "append comment to agent.pb.go" verify-proto \
    '^DRIFT: api/internal/grpc/rustagent/agent\.pb\.go$' make verify-proto
}

control_proto_source() {
  local case_dir
  case_dir="$(new_case proto-source)"
  printf '\nmessage NegativeControlOnly { string value = 1; }\n' >> "$case_dir/proto/kari/agent/v1/agent.proto"
  expect_failure "$case_dir" proto-source "append protobuf message without regeneration" verify-proto \
    '^DRIFT: agent/src/proto/agent_descriptor\.bin$' make verify-proto
}

control_repository_column() {
  local case_dir
  case_dir="$(new_case repository-column)"
  perl -pi -e 's/SELECT u\.id, u\.username/SELECT u.missing_column, u.username/' \
    "$case_dir/api/internal/db/postgres/user_repo.go"
  expect_failure "$case_dir" repository-column "replace users.id query with users.missing_column" verify-database \
    'column u\.missing_column does not exist' make verify-database
}

control_missing_migration() {
  local case_dir
  case_dir="$(new_case missing-migration)"
  mv "$case_dir/api/internal/db/migrations/000003_resources.sql" \
    "$case_dir/api/internal/db/migrations/000003_resources.sql.removed"
  expect_failure "$case_dir" missing-migration "rename 000003_resources.sql out of chain" migration-manifest \
    'migration manifest mismatch' go test -count=1 ./api/internal/db -run TestLoadMigrationPlan
}

control_migration_order() {
  local case_dir
  local temporary_file
  case_dir="$(new_case migration-order)"
  temporary_file="$case_dir/api/internal/db/migrations/ordering.swap"
  cp "$case_dir/api/internal/db/migrations/000002_identity.sql" "$temporary_file"
  cp "$case_dir/api/internal/db/migrations/000003_resources.sql" \
    "$case_dir/api/internal/db/migrations/000002_identity.sql"
  cp "$temporary_file" "$case_dir/api/internal/db/migrations/000003_resources.sql"
  expect_failure "$case_dir" migration-order "swap identity and resource migration bodies" verify-database \
    'apply migration 000002_identity\.sql:.*relation "users" does not exist' make verify-database
}

control_toolchain_conflict() {
  local case_dir
  case_dir="$(new_case toolchain-conflict)"
  printf '22.0.0\n' > "$case_dir/.nvmrc"
  expect_failure "$case_dir" toolchain-conflict "set .nvmrc to 22.0.0" verify-preflight \
    '\.nvmrc version mismatch' make verify-preflight
}

control_missing_dockerfile() {
  local case_dir
  case_dir="$(new_case missing-dockerfile)"
  perl -pi -e 's#dockerfile: api/Dockerfile#dockerfile: api/DoesNotExist#g' "$case_dir/docker-compose.yml"
  expect_failure "$case_dir" missing-dockerfile "replace API Dockerfile path in Compose" verify-topology \
    'Compose references nonexistent Dockerfile:.*api/DoesNotExist' make verify-topology
}

control_health_endpoint() {
  local case_dir
  case_dir="$(new_case health-endpoint)"
  perl -pi -e 's#router\.Get\("/health"#router.Get("/health-broken"#' \
    "$case_dir/api/internal/api/router/router.go"
  expect_failure "$case_dir" health-endpoint "rename /health route to /health-broken" health-contract \
    'liveness response = 404' go test -count=1 ./api/internal/api/router -run TestLiveness
}

control_frontend_stage() {
  local case_dir
  case_dir="$(new_case frontend-stage)"
  perl -ni -e 'print unless /^  verify_types$/' "$case_dir/scripts/verify-frontend.sh"
  expect_failure "$case_dir" frontend-stage "remove verify_types from frontend orchestration" verify-static \
    'frontend type-check stage missing' make verify-static
}

control_documented_path() {
  local case_dir
  case_dir="$(new_case documented-path)"
  perl -pi -e 's/cp \.env\.example \.env/cp missing.env .env/' "$case_dir/README.md"
  expect_failure "$case_dir" documented-path "replace .env.example with missing.env" verify-docs \
    'documented copy source does not exist: missing\.env' make verify-docs
}

control_stale_go() {
  local case_dir
  case_dir="$(new_case stale-go)"
  perl -pi -e 's/^go 1\.27\.0$/go 1.24.3/' "$case_dir/go.mod"
  expect_failure "$case_dir" stale-go "set go.mod to Go 1.24.3" verify-preflight \
    "go\.mod version mismatch: expected '1\.27\.0', observed '1\.24\.3'" make verify-preflight
}

control_stale_node() {
  local case_dir
  case_dir="$(new_case stale-node)"
  perl -0pi -e 's/"node": "24\.19\.0"/"node": "22.23.2"/' "$case_dir/frontend/package.json"
  expect_failure "$case_dir" stale-node "set frontend engines.node to 22.23.2" verify-preflight \
    "frontend Node engine version mismatch: expected '24\.19\.0', observed '22\.23\.2'" make verify-preflight
}

control_stale_npm() {
  local case_dir
  case_dir="$(new_case stale-npm)"
  perl -0pi -e 's/"npm": "11\.17\.0"/"npm": "10.9.8"/' \
    "$case_dir/frontend/package.json"
  expect_failure "$case_dir" stale-npm "set frontend engines.npm to 10.9.8" verify-preflight \
    "frontend npm engine version mismatch: expected '11\.17\.0', observed '10\.9\.8'" make verify-preflight
}

control_stale_rust() {
  local case_dir
  case_dir="$(new_case stale-rust)"
  perl -pi -e 's/channel = "1\.98\.0"/channel = "1.96.0"/' "$case_dir/rust-toolchain.toml"
  expect_failure "$case_dir" stale-rust "set rust-toolchain.toml to 1.96.0" verify-preflight \
    "Rust pin version mismatch: expected '1\.98\.0', observed '1\.96\.0'" make verify-preflight
}

control_mutable_action() {
  local case_dir
  case_dir="$(new_case mutable-action)"
  perl -pi -e 's#actions/checkout@[0-9a-f]{40}#actions/checkout\@v7#' \
    "$case_dir/.github/workflows/verify.yml"
  expect_failure "$case_dir" mutable-action "replace approved checkout SHA with mutable v7 tag" workflow-policy \
    'verify\.yml: job "verify" step "Checkout repository" action "actions/checkout@v7": external action reference must be a full 40-character SHA' \
    make verify-static
}

control_unused_action_policy() {
  local case_dir
  local temporary_policy
  case_dir="$(new_case unused-action-policy)"
  temporary_policy="$case_dir/.github/actions-policy.tmp"
  jq '.actions += [{
    "repository": "actions/cache",
    "release": "v6.1.0",
    "sha": "55cc8345863c7cc4c66a329aec7e433d2d1c52a9",
    "runtime": "javascript-node24",
    "minimum_major": 6,
    "allowed_paths": [""],
    "release_url": "https://github.com/actions/cache/releases/tag/v6.1.0"
  }]' "$case_dir/.github/actions-policy.json" > "$temporary_policy"
  mv "$temporary_policy" "$case_dir/.github/actions-policy.json"
  expect_failure "$case_dir" unused-action-policy "add valid unused actions/cache policy entry" workflow-policy-closure \
    'action policy entry "actions/cache" is unused: no active workflow references it' make verify-static
}

expect_failure() {
  local case_dir="$1"
  local control="$2"
  local mutation="$3"
  local expected_stage="$4"
  local expected_pattern="$5"
  shift 5
  local log_file="$RESULTS_DIR/${control}.log"
  local status
  local observed
  if (cd "$case_dir" && "$@") > "$log_file" 2>&1; then
    status=0
  else
    status=$?
  fi
  [[ "$status" -ne 0 ]] || fail "$control unexpectedly passed"
  observed="$(rg -m 1 "$expected_pattern" "$log_file" || true)"
  [[ -n "$observed" ]] || fail "$control failed for the wrong reason; inspect $log_file"
  observed="$(printf '%s' "$observed" | tr '\t' ' ')"
  assert_source_unchanged
  printf '%s\t%s\t%s\t%s\t%d\t%s\t%s\n' \
    "$control" "$mutation" "$expected_stage" "$observed" "$status" "$SOURCE_IDENTITY" "$*" >> "$RESULTS_FILE"
}

new_case() {
  local case_dir="$TEMP_ROOT/$1"
  mkdir -p "$case_dir"
  cp -R "$PRISTINE_DIR/." "$case_dir/"
  printf '%s\n' "$case_dir"
}

assert_source_unchanged() {
  local current_identity
  current_identity="$(source_identity)"
  [[ "$current_identity" == "$SOURCE_IDENTITY" ]] || fail "source tree changed during negative controls"
}

source_identity() {
  "$ROOT_DIR/scripts/source-identity.sh" \
    --manifest "$TEMP_ROOT/protected-source-identity-v2.manifest" \
    --format sha-only
}

cleanup() {
  rm -rf "$TEMP_ROOT"
}

fail() {
  echo "ERROR: $1" >&2
  exit 1
}

main "$@"
