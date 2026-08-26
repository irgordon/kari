#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly ACTIVE_DOCS=(README.md DEVELOPMENT.md CONTRIBUTING.md docs/OPERATIONS.md docs/SOURCE_IDENTITY.md)

main() {
  verify_required_references
  reject_retired_references
  verify_script_commands
  verify_make_commands
  verify_copy_sources
  verify_markdown_links
}

verify_copy_sources() {
  local source_path
  while IFS= read -r source_path; do
    [[ -e "$ROOT_DIR/$source_path" ]] || fail "documented copy source does not exist: $source_path"
  done < <(extract_docs | sed -n 's/^[[:space:]]*cp[[:space:]]\([^[:space:]]*\).*/\1/p' | sort -u)
}

verify_required_references() {
  require_text 'make verify'
  require_text 'docker-compose.yml'
  require_text 'deploy/systemd/kari-agent.service'
  require_text '/health'
  require_text '/ready'
  require_text 'kari-source-identity-v2'
}

reject_retired_references() {
  local pattern
  for pattern in 'docker-compose.prod.yml' './dev.sh' './up.sh' 'Svelte' 'build-passing' 'google.com/search'; do
    if search_docs "$pattern" >/dev/null; then
      fail "retired documentation reference found: $pattern"
    fi
  done
}

verify_script_commands() {
  local path
  while IFS= read -r path; do
    [[ -x "$ROOT_DIR/$path" ]] || fail "documented script is missing or not executable: $path"
  done < <(extract_docs | sed -n 's#^[[:space:]]*\(\./scripts/[A-Za-z0-9_./-]*\.sh\).*#\1#p' | sed 's#^\./##' | sort -u)
}

verify_make_commands() {
  local target
  while IFS= read -r target; do
    make -C "$ROOT_DIR" --dry-run "$target" >/dev/null || fail "documented make target does not exist: $target"
  done < <(extract_docs | sed -n 's/^[[:space:]]*make[[:space:]]\([A-Za-z0-9_-]*\).*/\1/p' | sort -u)
}

verify_markdown_links() {
  local path
  while IFS= read -r path; do
    [[ -e "$ROOT_DIR/$path" ]] || fail "documented relative link does not exist: $path"
  done < <(extract_docs | sed -n 's/.*](\([^):#?]*\)).*/\1/p' | grep -v '^$' | sort -u)
}

require_text() {
  search_docs "$1" >/dev/null || fail "required documentation reference missing: $1"
}

search_docs() {
  (cd "$ROOT_DIR" && rg -n --fixed-strings "$1" "${ACTIVE_DOCS[@]}")
}

extract_docs() {
  local document
  for document in "${ACTIVE_DOCS[@]}"; do
    sed -n '1,$p' "$ROOT_DIR/$document"
  done
}

fail() {
  echo "ERROR: $1" >&2
  exit 1
}

main "$@"
