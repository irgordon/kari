#!/usr/bin/env bash
set -euo pipefail

# ------------------------------------------------------------------------------
# Karı Local Bootstrap Script
# ------------------------------------------------------------------------------
# Goal:
#   - Give developers one command for first-run setup
#   - Keep config/proto optional for normal contributors
#   - Validate deterministic local workflow
# ------------------------------------------------------------------------------

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERROR: '$1' is required but not installed."
    echo "Install it and re-run ./scripts/bootstrap.sh"
    exit 1
  fi
}

echo "==> Karı Bootstrap"
echo "    repo: $ROOT_DIR"
echo

echo "==> Checking required tools"
need make
need go
need cargo
need npm
need docker
need docker-compose
echo "    ✓ required tools present"
echo

echo "==> Checking optional proto toolchain"
if command -v protoc >/dev/null 2>&1 && command -v protoc-gen-go >/dev/null 2>&1 && command -v protoc-gen-go-grpc >/dev/null 2>&1; then
  echo "    ✓ protoc toolchain detected (proto checks enabled)"
else
  echo "    ℹ protoc/protoc-gen-go/protoc-gen-go-grpc not fully installed"
  echo "      This is okay unless you are modifying .proto definitions."
fi
echo

echo "==> Running one-command deterministic workflow"
(
  cd "$ROOT_DIR"
  make dev
)
echo "    ✓ make dev complete"
echo

echo "==> Smoke check: healthcheck binary build"
(
  cd "$ROOT_DIR"
  go build -o /tmp/kari-healthcheck ./api/cmd/healthcheck
)
echo "    ✓ healthcheck binary built: /tmp/kari-healthcheck"
echo

echo "==> Bootstrap complete"
echo "Try these next commands:"
echo "  make up"
echo "  make logs"
echo "  make down"
