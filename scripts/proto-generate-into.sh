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
  source "$ROOT_DIR/toolchain.env"
  command -v protoc >/dev/null
  command -v protoc-gen-go >/dev/null
  command -v protoc-gen-go-grpc >/dev/null
  require_version "protoc" "libprotoc $PROTOC_DISPLAY_VERSION" "$(protoc --version)"
  require_version "protoc-gen-go" "protoc-gen-go $PROTOC_GEN_GO_VERSION" "$(protoc-gen-go --version)"
  require_version "protoc-gen-go-grpc" "protoc-gen-go-grpc ${PROTOC_GEN_GO_GRPC_VERSION#v}" "$(protoc-gen-go-grpc --version)"
}

require_version() {
  if [[ "$2" != "$3" ]]; then
    echo "ERROR: $1 version mismatch: expected '$2', observed '$3'" >&2
    exit 1
  fi
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
