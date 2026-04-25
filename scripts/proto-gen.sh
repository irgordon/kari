#!/usr/bin/env bash
set -euo pipefail

PROTO_SRC="proto/kari/agent/v1/agent.proto"
GO_OUT="api/internal/grpc/rustagent"
RUST_OUT="agent/src/proto"

# Optional proto toolchain: skip if missing
missing=0
if ! command -v protoc >/dev/null 2>&1; then
  echo "⚠️  Skipping proto generation: protoc not found."
  missing=1
fi
if ! command -v protoc-gen-go >/dev/null 2>&1; then
  echo "⚠️  Skipping proto generation: protoc-gen-go not found."
  missing=1
fi
if ! command -v protoc-gen-go-grpc >/dev/null 2>&1; then
  echo "⚠️  Skipping proto generation: protoc-gen-go-grpc not found."
  missing=1
fi
if [[ "$missing" -eq 1 ]]; then
  echo "ℹ️  Proto generation skipped. Install tools only when modifying API definitions."
  exit 0
fi

MODULE_PATH="$(go list -m -f '{{.Path}}' 2>/dev/null || true)"
if [[ -z "$MODULE_PATH" ]]; then
  echo "❌ Unable to resolve Go module path."
  exit 1
fi

EXPECTED_GO_PACKAGE="${MODULE_PATH}/api/internal/grpc/rustagent"
echo "Generating gRPC stubs..."
echo "  module:     $MODULE_PATH"
echo "  go_package: ${EXPECTED_GO_PACKAGE};rustagent"

mkdir -p "$GO_OUT" "$RUST_OUT"
rm -f "$GO_OUT"/*.go
rm -f "$RUST_OUT/agent_descriptor.bin"

echo "  → Go stubs"
protoc --proto_path=. \
  --go_out=. --go_opt=module="$MODULE_PATH" \
  --go-grpc_out=. --go-grpc_opt=module="$MODULE_PATH" \
  "$PROTO_SRC"

echo "  → Rust descriptor"
protoc --proto_path=. "$PROTO_SRC" \
  --descriptor_set_out="$RUST_OUT/agent_descriptor.bin"

if [[ "$OSTYPE" == "linux-gnu"* ]]; then
  chown -R 1001:1001 "$GO_OUT" || true
fi

echo "Done."
echo "Go:   $GO_OUT"
echo "Rust: $RUST_OUT/agent_descriptor.bin"