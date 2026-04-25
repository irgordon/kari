#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

chmod +x scripts/proto-gen.sh
./scripts/proto-gen.sh

GENERATED_PATHS=(api/internal/grpc/rustagent agent/src/proto/agent_descriptor.bin)

if ! git diff --exit-code -- "${GENERATED_PATHS[@]}" >/dev/null; then
  echo "❌ Protobuf outputs are out of date. Run: make proto"
  exit 1
fi

if [[ -n "$(git status --porcelain --untracked-files=all -- "${GENERATED_PATHS[@]}")" ]]; then
  echo "❌ Protobuf outputs are out of date. Run: make proto"
  exit 1
fi
echo "✅ Protobuf outputs are up to date."
