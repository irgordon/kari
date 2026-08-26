#!/usr/bin/env bash
set -euo pipefail

readonly FRONTEND_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../frontend" && pwd)"

main() {
  verify_toolchain
  install_dependencies
  verify_lints
  verify_types
  verify_production_build
}

verify_toolchain() {
  source "$FRONTEND_DIR/../toolchain.env"
  [[ "$(node --version)" == "v${NODE_VERSION}" ]] || fail "Node.js toolchain mismatch"
  [[ "$(npm --version)" == "$NPM_VERSION" ]] || fail "npm toolchain mismatch"
}

fail() {
  echo "ERROR: $1" >&2
  exit 1
}

install_dependencies() {
  (cd "$FRONTEND_DIR" && npm ci)
}

verify_lints() {
  (cd "$FRONTEND_DIR" && npm run lint)
}

verify_types() {
  (cd "$FRONTEND_DIR" && npm run typecheck)
}

verify_production_build() {
  (cd "$FRONTEND_DIR" && npm run build)
}

main "$@"
