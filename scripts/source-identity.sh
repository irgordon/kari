#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

main() {
  (cd "$ROOT_DIR" && go run ./api/cmd/source-identity --repo "$ROOT_DIR" "$@")
}

main "$@"
