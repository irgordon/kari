#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

main() {
  verify_toolchain
  verify_formatting
  verify_static_analysis
  verify_tests
  verify_race_safety
}

verify_toolchain() {
  source "$ROOT_DIR/toolchain.env"
  [[ "$(go version)" == "go version go${GO_VERSION} "* ]] || fail "Go toolchain mismatch"
}

verify_formatting() {
  local unformatted
  unformatted="$(cd "$ROOT_DIR" && gofmt -l $(git ls-files '*.go'))"
  [[ -z "$unformatted" ]] || fail "Go files require gofmt:\n$unformatted"
}

verify_static_analysis() {
  (cd "$ROOT_DIR" && go vet ./api/...)
}

verify_tests() {
  (cd "$ROOT_DIR" && go test -count=1 ./api/...)
}

verify_race_safety() {
  (cd "$ROOT_DIR" && go test -race -count=1 ./api/...)
}

fail() {
  printf '%b\n' "ERROR: $1" >&2
  exit 1
}

main "$@"
