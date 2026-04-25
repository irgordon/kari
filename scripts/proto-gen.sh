#!/usr/bin/env bash

# Karı Orchestration Engine - Proto Generation Utility
# 🛡️ SLA: Synchronize gRPC stubs for Brain (Go) and Muscle (Rust)
# 🛡️ Zero-Trust: Enforce strict package boundaries

set -euo pipefail

# Path Definitions
PROTO_SRC="proto/kari/agent/v1/agent.proto"
GO_OUT="api/internal/grpc/rustagent"
RUST_OUT="agent/src/proto"

if ! command -v protoc >/dev/null 2>&1; then
  echo "❌ protoc is required but was not found on PATH"
  exit 1
fi

if ! command -v protoc-gen-go >/dev/null 2>&1; then
  echo "❌ protoc-gen-go is required but was not found on PATH"
  echo "   Install: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"
  exit 1
fi

if ! command -v protoc-gen-go-grpc >/dev/null 2>&1; then
  echo "❌ protoc-gen-go-grpc is required but was not found on PATH"
  echo "   Install: go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"
  exit 1
fi

echo "🧬 Karı Panel: Commencing gRPC stub generation..."

# 1. 🛡️ Clean existing stubs to prevent stale artifact leakage
mkdir -p "$GO_OUT"
mkdir -p "$RUST_OUT"
rm -f "$GO_OUT"/*.go
rm -f "$RUST_OUT/agent_descriptor.bin"

# 2. 🧠 Generate Go Stubs (The Brain)
# `go_package = kari/api/internal/grpc/rustagent` + `module=kari`
# writes to api/internal/grpc/rustagent/*.pb.go deterministically.
echo "  ➜ Generating Go stubs..."
protoc --proto_path=. \
  --go_out=. --go_opt=module=kari \
  --go-grpc_out=. --go-grpc_opt=module=kari \
  "$PROTO_SRC"

# 3. ⚙️ Generate Rust Descriptor (The Muscle)
echo "  ➜ Verifying Rust proto boundaries..."
protoc --proto_path=. "$PROTO_SRC" --descriptor_set_out="$RUST_OUT/agent_descriptor.bin"

# 4. 🛡️ Permission Hardening
# Ensure the generated Go code matches our API UID (1001) in Docker
if [[ "$OSTYPE" == "linux-gnu"* ]]; then
  chown -R 1001:1001 "$GO_OUT" || true
fi

echo "✅ Generation Complete."
echo "--------------------------------------------------"
echo "🧠 Go: $GO_OUT"
echo "⚙️  Rust: $RUST_OUT/agent_descriptor.bin (updated)"
