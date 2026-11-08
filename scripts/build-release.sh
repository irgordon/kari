#!/usr/bin/env bash
set -euo pipefail
readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly VERSION="${1:?usage: build-release.sh VERSION}"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$ ]] || { echo 'Invalid release version' >&2; exit 1; }
source "$ROOT_DIR/toolchain.env"
readonly OUT="$ROOT_DIR/.artifacts/releases/$VERSION"
[[ ! -e "$OUT" ]] || { echo 'Release directory already exists; artifacts are immutable' >&2; exit 1; }
mkdir -p "$OUT"
identity="$("$ROOT_DIR/scripts/source-identity.sh" --manifest "$OUT/source.manifest")"
tree="$(printf '%s' "$identity" | jq -r .git_tree_id)"
case "$(docker info --format '{{.Architecture}}')" in
  aarch64|arm64) ARCH=arm64; PROTO_ARCH=aarch_64; PROTO_SHA="$PROTOC_LINUX_ARM64_SHA256" ;;
  x86_64|amd64) ARCH=amd64; PROTO_ARCH=x86_64; PROTO_SHA="$PROTOC_LINUX_X86_64_SHA256" ;;
  *) echo 'Unsupported Docker architecture' >&2; exit 1 ;;
esac
readonly PROTO_DIR="$ROOT_DIR/.artifacts/protoc-linux-$ARCH"
mkdir -p "$PROTO_DIR"
curl --fail --silent --show-error --location "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-${PROTO_ARCH}.zip" --output "$PROTO_DIR/archive.zip"
[[ "$(shasum -a 256 "$PROTO_DIR/archive.zip" | awk '{print $1}')" == "$PROTO_SHA" ]] || { echo 'protoc checksum mismatch' >&2; exit 1; }
unzip -q -o "$PROTO_DIR/archive.zip" -d "$PROTO_DIR"
docker build --build-arg "GO_BUILDER_IMAGE=$GO_BUILDER_IMAGE" --tag "kari-api:$VERSION" --file "$ROOT_DIR/api/Dockerfile" "$ROOT_DIR"
docker build --build-arg "NODE_BUILDER_IMAGE=$NODE_BUILDER_IMAGE" --build-arg "FRONTEND_RUNTIME_IMAGE=$FRONTEND_RUNTIME_IMAGE" --tag "kari-frontend:$VERSION" "$ROOT_DIR/frontend"
container="$(docker create "kari-api:$VERSION")"
trap 'docker rm "$container" >/dev/null' EXIT
docker cp "$container:/app/kari-admin" "$OUT/kari-admin"
lock_hash="$(shasum -a 256 "$ROOT_DIR/agent/Cargo.lock" | cut -c 1-12)"
docker run --rm --volume "$ROOT_DIR/agent:/src/agent:ro" --volume "$ROOT_DIR/proto:/src/proto:ro" --volume "$OUT:/out" --volume "$PROTO_DIR:/protoc:ro" \
  --volume "kari-rust-${RUST_VERSION}-${lock_hash}:/target" --volume "kari-cargo-${RUST_VERSION}-${lock_hash}:/usr/local/cargo/registry" \
  --env PROTOC=/protoc/bin/protoc --env CARGO_TARGET_DIR=/target --workdir /src/agent "$RUST_BUILDER_IMAGE" \
  sh -c 'cargo build --release --locked && cp /target/release/kari-agent /out/kari-agent'
docker pull "$POSTGRES_IMAGE"
docker save "kari-api:$VERSION" "kari-frontend:$VERSION" "$POSTGRES_IMAGE" --output "$OUT/images.tar"
cp "$ROOT_DIR/docker-compose.yml" "$ROOT_DIR/toolchain.env" "$OUT/"
cp "$ROOT_DIR/deploy/systemd/kari-agent.service" "$ROOT_DIR/deploy/systemd/kari-panel.service" "$OUT/"
cp "$ROOT_DIR/frontend/nginx.conf" "$OUT/frontend.conf"
after="$("$ROOT_DIR/scripts/source-identity.sh" --manifest "$OUT/source-after.manifest")"
[[ "$(printf '%s' "$after" | jq -r .git_tree_id)" == "$tree" ]] || { echo 'Source changed during build; no release manifest produced' >&2; exit 1; }
rm "$OUT/source-after.manifest"
(cd "$ROOT_DIR" && go run ./api/cmd/kari-admin bundle --bundle "$OUT" --version "$VERSION" --arch "$ARCH" --tree "$tree")
shasum -a 256 "$OUT/manifest.json"
