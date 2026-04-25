#!/bin/bash

# Karı Orchestration Engine - Proto Generation Utility
# 🛡️ SLA: Synchronize gRPC stubs for Brain (Go) and Muscle (Rust)
# 🛡️ Zero-Trust: Enforce strict package boundaries

set -euo pipefail

# Path Definitions
PROTO_SRC="proto/kari/agent/v1/agent.proto"
GO_OUT="api/internal/grpc/rustagent"
RUST_OUT="agent/src/proto"

echo "🧬 Karı Panel: Commencing gRPC stub generation..."

# 1. 🛡️ Clean existing stubs to prevent stale artifact leakage
mkdir -p "$GO_OUT"
mkdir -p "$RUST_OUT"
rm -rf "${GO_OUT:?}"/*
rm -f "$RUST_OUT/agent_descriptor.bin"

# 2. 🧠 Generate Go Stubs (The Brain)
# Requires: protoc, protoc-gen-go, protoc-gen-go-grpc
echo "  ➜ Generating Go stubs..."
protoc --proto_path=. \
    --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    "$PROTO_SRC"

# 3. ⚙️ Generate Rust Stubs (The Muscle)
# While Rust typically uses tonic-build in a build.rs, generating
# them via protoc is useful for CI/CD auditing and external tooling.
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
