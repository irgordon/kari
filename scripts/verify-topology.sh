#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

main() {
  validate_compose_configuration
  validate_compose_build_references
  validate_topology_files
  validate_native_agent_boundary
  "$ROOT_DIR/scripts/topology-smoke.sh"
}

validate_compose_build_references() {
  local build_path
  while IFS= read -r build_path; do
    [[ -f "$build_path" ]] || fail "Compose references nonexistent Dockerfile: $build_path"
  done < <("$ROOT_DIR/scripts/compose.sh" config --format json | \
    jq -r '.services[] | select(.build) | .build.context + "/" + .build.dockerfile')
}

validate_compose_configuration() {
  "$ROOT_DIR/scripts/compose.sh" config --quiet
}

validate_topology_files() {
  require_file api/Dockerfile
  require_file frontend/Dockerfile
  require_file frontend/nginx.conf
  require_file deploy/systemd/kari-agent.service
  require_file deploy/systemd/agent.env.example
}

validate_native_agent_boundary() {
  if "$ROOT_DIR/scripts/compose.sh" config --services | grep -qx 'agent'; then
    fail "the privileged native agent must not be a Compose service"
  fi
  require_unit_setting 'ExecStart=/usr/local/libexec/kari/kari-agent'
  require_unit_setting 'User=root'
  require_unit_setting 'EnvironmentFile=/etc/kari/agent.env'
}

require_file() {
  [[ -f "$ROOT_DIR/$1" ]] || fail "required topology file not found: $1"
}

require_unit_setting() {
  grep -Fqx "$1" "$ROOT_DIR/deploy/systemd/kari-agent.service" || fail "systemd setting missing: $1"
}

fail() {
  echo "ERROR: $1" >&2
  exit 1
}

main "$@"
