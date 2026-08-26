#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

main() {
  run_compose "$@"
}

run_compose() {
  local -a command_args
  command_args=(--env-file "$ROOT_DIR/toolchain.env")
  if [[ -f "$ROOT_DIR/.env" ]]; then
    command_args+=(--env-file "$ROOT_DIR/.env")
  fi
  command_args+=(-f "$ROOT_DIR/docker-compose.yml" "$@")
  compose_command "${command_args[@]}"
}

compose_command() {
  if docker compose version >/dev/null 2>&1; then
    docker compose "$@"
    return
  fi
  command -v docker-compose >/dev/null 2>&1 || fail
  docker-compose "$@"
}

fail() {
  echo "ERROR: Docker Compose is required" >&2
  exit 1
}

main "$@"
