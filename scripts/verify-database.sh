#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly CONTAINER_NAME="kari-contract-${$}"
readonly DATABASE_PASSWORD="kari_contract_password"

main() {
  load_versions
  trap cleanup EXIT
  start_postgres
  wait_for_postgres
  verify_migrations
  verify_repository_contracts
}

load_versions() {
  set -a
  source "$ROOT_DIR/toolchain.env"
  set +a
}

start_postgres() {
  docker run --detach --rm --name "$CONTAINER_NAME" \
    --env POSTGRES_DB=kari_contract \
    --env POSTGRES_USER=kari \
    --env POSTGRES_PASSWORD="$DATABASE_PASSWORD" \
    --publish 127.0.0.1::5432 \
    "$POSTGRES_IMAGE" >/dev/null
}

wait_for_postgres() {
  local attempt
  for attempt in $(seq 1 60); do
    if docker exec "$CONTAINER_NAME" pg_isready -U kari -d kari_contract >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done
  docker logs "$CONTAINER_NAME" >&2
  fail "PostgreSQL did not become ready"
}

verify_migrations() {
  local database_url
  database_url="$(contract_database_url)"
  (cd "$ROOT_DIR" && DATABASE_URL="$database_url" go run ./api/cmd/migrate)
  (cd "$ROOT_DIR" && DATABASE_URL="$database_url" go run ./api/cmd/migrate)
}

verify_repository_contracts() {
  local database_url
  database_url="$(contract_database_url)"
  (cd "$ROOT_DIR" && KARI_TEST_DATABASE_URL="$database_url" \
    go test -tags=integration -count=1 ./api/internal/db/contract)
}

contract_database_url() {
  local port
  port="$(docker port "$CONTAINER_NAME" 5432/tcp | sed 's/.*://')"
  printf 'postgres://kari:%s@127.0.0.1:%s/kari_contract?sslmode=disable\n' "$DATABASE_PASSWORD" "$port"
}

cleanup() {
  docker rm --force "$CONTAINER_NAME" >/dev/null 2>&1 || true
}

fail() {
  echo "ERROR: $1" >&2
  exit 1
}

main "$@"
