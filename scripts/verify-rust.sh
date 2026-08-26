#!/usr/bin/env bash
set -euo pipefail

readonly AGENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../agent" && pwd)"

main() {
  verify_toolchain
  verify_formatting
  verify_compilation
  verify_lints
  verify_tests
}

verify_toolchain() {
  source "$AGENT_DIR/../toolchain.env"
  [[ "$(rustup run "$RUST_VERSION" rustc --version)" == "rustc ${RUST_VERSION} "* ]] || fail "Rust toolchain mismatch"
}

fail() {
  echo "ERROR: $1" >&2
  exit 1
}

verify_formatting() {
  (cd "$AGENT_DIR" && rustup run "$RUST_VERSION" cargo fmt -- --check)
}

verify_compilation() {
  (cd "$AGENT_DIR" && rustup run "$RUST_VERSION" cargo build --locked --all-targets)
}

verify_lints() {
  (cd "$AGENT_DIR" && rustup run "$RUST_VERSION" cargo clippy --locked --all-targets --all-features -- \
    -D warnings -A clippy::result-large-err)
}

verify_tests() {
  (cd "$AGENT_DIR" && rustup run "$RUST_VERSION" cargo test --locked)
}

main "$@"
