#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly OUTPUT_ROOT="${1:?output root is required}"
readonly MODULE_PATH="github.com/irgordon/kari"
readonly PROTO_SOURCE="proto/kari/agent/v1/agent.proto"

main() {
  require_generators
  prepare_output_directories
  generate_go_outputs
  generate_rust_descriptor
}

require_generators() {
  command -v protoc >/dev/null
  command -v protoc-gen-go >/dev/null
  command -v protoc-gen-go-grpc >/dev/null
}

prepare_output_directories() {
  mkdir -p "$OUTPUT_ROOT/api/internal/grpc/rustagent"
  mkdir -p "$OUTPUT_ROOT/agent/src/proto"
}

generate_go_outputs() {
  (cd "$ROOT_DIR" && protoc --proto_path=. \
    --go_out="$OUTPUT_ROOT" --go_opt=module="$MODULE_PATH" \
    --go-grpc_out="$OUTPUT_ROOT" --go-grpc_opt=module="$MODULE_PATH" \
    "$PROTO_SOURCE")
}

generate_rust_descriptor() {
  (cd "$ROOT_DIR" && protoc --proto_path=. "$PROTO_SOURCE" \
    --descriptor_set_out="$OUTPUT_ROOT/agent/src/proto/agent_descriptor.bin")
}

main "$@"
