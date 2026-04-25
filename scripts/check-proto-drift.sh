#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

chmod +x scripts/proto-gen.sh
./scripts/proto-gen.sh

if ! git diff --exit-code -- api/internal/grpc/rustagent agent/src/proto/agent_descriptor.bin; then
  echo "❌ Protobuf outputs are out of date. Run: make proto"
  exit 1
fi

echo "✅ Protobuf outputs are up to date."
