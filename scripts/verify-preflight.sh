#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

main() {
  load_versions
  require_commands
  verify_exact_toolchains
  verify_toolchain_consumers
  report_container_tools
}

load_versions() {
  set -a
  source "$ROOT_DIR/toolchain.env"
  set +a
}

require_commands() {
  local command_name
  for command_name in go rustup node npm protoc protoc-gen-go protoc-gen-go-grpc docker jq; do
    require_command "$command_name"
  done
  "$ROOT_DIR/scripts/compose.sh" version >/dev/null
}

verify_toolchain_consumers() {
  require_equal "go.mod" "$GO_VERSION" "$(awk '/^go / {print $2}' "$ROOT_DIR/go.mod")"
  require_equal ".nvmrc" "$NODE_VERSION" "$(tr -d '[:space:]' < "$ROOT_DIR/.nvmrc")"
  require_equal "Rust pin" "$RUST_VERSION" "$(sed -n 's/^channel = "\([^"]*\)"/\1/p' "$ROOT_DIR/rust-toolchain.toml")"
  require_equal "frontend Node engine" "$NODE_VERSION" "$(jq -r '.engines.node' "$ROOT_DIR/frontend/package.json")"
  require_equal "frontend npm engine" "$NPM_VERSION" "$(jq -r '.engines.npm' "$ROOT_DIR/frontend/package.json")"
  require_equal "frontend package manager" "npm@$NPM_VERSION" "$(jq -r '.packageManager' "$ROOT_DIR/frontend/package.json")"
  require_digest "$POSTGRES_IMAGE"
  require_digest "$GO_BUILDER_IMAGE"
  require_digest "$NODE_BUILDER_IMAGE"
  require_digest "$FRONTEND_RUNTIME_IMAGE"
}

require_digest() {
  [[ "$1" == *@sha256:* ]] || fail "container image is not pinned by digest: $1"
}

verify_exact_toolchains() {
  require_exact "Go" "go version go${GO_VERSION} " "$(go version)"
  require_exact "Rust" "rustc ${RUST_VERSION} " "$(rustup run "$RUST_VERSION" rustc --version)"
  require_equal "Node.js" "v${NODE_VERSION}" "$(node --version)"
  require_equal "npm" "$NPM_VERSION" "$(npm --version)"
  require_equal "protoc" "libprotoc ${PROTOC_DISPLAY_VERSION}" "$(protoc --version)"
  require_equal "protoc-gen-go" "protoc-gen-go ${PROTOC_GEN_GO_VERSION}" "$(protoc-gen-go --version)"
  require_equal "protoc-gen-go-grpc" "protoc-gen-go-grpc ${PROTOC_GEN_GO_GRPC_VERSION#v}" "$(protoc-gen-go-grpc --version)"
}

report_container_tools() {
  local docker_version
  local compose_version
  docker_version="$(docker --version | sed -n 's/^Docker version \([^,]*\).*/\1/p')"
  compose_version="$("$ROOT_DIR/scripts/compose.sh" version --short 2>/dev/null || \
    "$ROOT_DIR/scripts/compose.sh" version | awk '{print $NF}')"
  require_minimum "Docker" "$DOCKER_MIN_VERSION" "$docker_version"
  require_minimum "Compose" "$COMPOSE_MIN_VERSION" "$compose_version"
  docker info --format 'Docker server {{.ServerVersion}}'
  echo "Docker ${docker_version}; Compose ${compose_version}"
}

require_minimum() {
  version_at_least "$3" "$2" || fail "$1 version mismatch: minimum '$2', observed '$3'"
}

version_at_least() {
  local observed_major observed_minor observed_patch minimum_major minimum_minor minimum_patch
  IFS=. read -r observed_major observed_minor observed_patch <<< "${1%%-*}"
  IFS=. read -r minimum_major minimum_minor minimum_patch <<< "${2%%-*}"
  (( observed_major > minimum_major )) && return 0
  (( observed_major < minimum_major )) && return 1
  (( observed_minor > minimum_minor )) && return 0
  (( observed_minor < minimum_minor )) && return 1
  (( ${observed_patch:-0} >= ${minimum_patch:-0} ))
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

require_equal() {
  [[ "$3" == "$2" ]] || fail "$1 version mismatch: expected '$2', observed '$3'"
}

require_exact() {
  [[ "$3" == "$2"* ]] || fail "$1 version mismatch: expected prefix '$2', observed '$3'"
}

fail() {
  echo "ERROR: $1" >&2
  exit 1
}

main "$@"
