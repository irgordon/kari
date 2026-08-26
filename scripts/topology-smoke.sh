#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly RUNTIME_DIR="$ROOT_DIR/.runtime/agent"

main() {
  configure_smoke_environment
  trap cleanup EXIT
  start_topology
  verify_topology_health
  echo "Topology healthy; privileged native agent was not started by this smoke test."
}

configure_smoke_environment() {
  set -a
  source "$ROOT_DIR/toolchain.env"
  set +a
  mkdir -p "$RUNTIME_DIR"
  export COMPOSE_PROJECT_NAME="kari-smoke-${$}"
  export KARI_AGENT_SOCKET_DIR="$RUNTIME_DIR"
  export KARI_REQUIRE_AGENT=false
  export KARI_API_PORT=0
  export KARI_FRONTEND_PORT=0
}

start_topology() {
  docker build --build-arg "GO_BUILDER_IMAGE=$GO_BUILDER_IMAGE" \
    --tag kari-api:local --file "$ROOT_DIR/api/Dockerfile" "$ROOT_DIR"
  "$ROOT_DIR/scripts/compose.sh" build frontend
  "$ROOT_DIR/scripts/compose.sh" up --no-build --detach
}

verify_topology_health() {
  local api_port
  local frontend_port
  api_port="$(wait_for_service_port api 8080)"
  frontend_port="$(wait_for_service_port frontend 8080)"
  wait_for_endpoint "http://127.0.0.1:${api_port}/health"
  wait_for_endpoint "http://127.0.0.1:${api_port}/ready"
  wait_for_endpoint "http://127.0.0.1:${frontend_port}/health"
}

wait_for_service_port() {
  local service="$1"
  local target_port="$2"
  local attempt
  local published
  for attempt in $(seq 1 120); do
    published="$("$ROOT_DIR/scripts/compose.sh" port "$service" "$target_port" 2>/dev/null || true)"
    if [[ -n "$published" ]]; then
      printf '%s\n' "${published##*:}"
      return
    fi
    sleep 1
  done
  fail "no published port for $service"
}

wait_for_endpoint() {
  local endpoint="$1"
  local attempt
  for attempt in $(seq 1 120); do
    if curl --fail --silent --show-error "$endpoint" >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done
  "$ROOT_DIR/scripts/compose.sh" ps --all >&2 || true
  "$ROOT_DIR/scripts/compose.sh" logs >&2 || true
  fail "endpoint did not become healthy: $endpoint"
}

cleanup() {
  "$ROOT_DIR/scripts/compose.sh" down --volumes --remove-orphans >/dev/null 2>&1 || true
}

fail() {
  echo "ERROR: $1" >&2
  exit 1
}

main "$@"
