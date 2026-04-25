# Project State Review & Validation (2026-04-25)

## Scope
This review validates the repository state after the prior phase recommendations from `docs/PROJECT_STATE_REVIEW_2026-04-24.md`, using local build/test checks and static repo inspection.

## Validation Checks Run

1. `go test ./...` (repo root)
2. `cargo test` (`agent/`)
3. `npm --prefix frontend run test -- --run`
4. `npm --prefix frontend run check`

## Current State Summary

### 1) Backend (Go API) remains blocked for reproducible local builds

`go test ./...` still fails early due to:
- missing module dependencies from `go.mod`/`go.sum` state (many packages unresolved), and
- unresolved internal gRPC import path (`kari/api/internal/grpc/rustagent`).

This confirms the same core blocker identified previously: backend dependency + protobuf integration is still not fully normalized for clean-checkout validation.

### 2) Agent (Rust) is healthy and test-passing

`cargo test` in `agent/` passes with 18/18 tests green. The agent remains the most stable and verifiable subsystem.

Observed warnings are non-blocking but indicate cleanup debt (unused imports/traits/fields).

### 3) Frontend verification remains environment-fragile

Frontend test/check commands fail in this clean checkout because frontend dependencies have not been installed yet (`frontend/node_modules` is missing), so package-provided binaries such as `vitest` and `svelte-kit` are not available until `npm ci` is run.

This indicates the repo still lacks a mandatory bootstrap/verify path that installs frontend dependencies before checks run.

### 4) Baseline orchestration maturity is still partial

The root `Makefile` still focuses on runtime lifecycle (`deploy`, `build`, `up`) and protobuf generation, but no single `verify` target exists for deterministic local CI-like validation across all layers.

## Delta vs Previous Review (2026-04-24)

- **No material change** in the highest-priority blocker (Go backend reproducibility).
- **Rust agent stability remains strong.**
- **Frontend checks are still not turnkey from current checkout state.**

Conclusion: the project is still in a partially integrated state, with the prior review's "Phase 0 goals" now represented by the immediate Phase A work and not yet fully completed.

---

## Recommended Next Phases

### Phase A (Immediate: 1-2 days) — Reproducible Backend Recovery

Goal: make `go test ./...` deterministic from clean checkout.

Actions:
1. Finalize canonical Go module strategy (single module root and committed lockfile artifacts).
2. Run dependency resolution and commit `go.sum` with pinned versions.
3. Regenerate protobuf/gRPC artifacts and align internal import path references so `kari/api/internal/grpc/rustagent` resolves consistently.
4. Add a guard check in CI that fails if generated proto outputs drift.

Exit criteria:
- `go test ./...` executes without module/proto import failures on fresh clone.

### Phase B (Short: 2-4 days) — Unified Developer Validation Path

Goal: one command to validate all stacks before merge.

Actions:
1. Add `make verify` target that runs:
   - Go test/build checks,
   - Rust fmt/clippy/test,
   - Frontend install + check + test.
2. Ensure frontend verify step performs dependency bootstrap (`npm ci`) before test/check in CI.
3. Document the exact local preflight workflow in `README.md` and `DEVELOPMENT.md`.

Exit criteria:
- New contributor can run one command from a clean checkout and get deterministic pass/fail feedback.

### Phase C (Medium: 1-2 weeks) — Integration Reliability Hardening

Goal: reduce deployment/runtime integration risk.

Actions:
1. Add API↔Agent integration tests for happy-path and failure-path gRPC interactions.
2. Implement deterministic rollback/cleanup assertions for deployment failures.
3. Address concurrency scaling debt from prior audit notes (deployment queue bottlenecks, worker fan-out controls).

Exit criteria:
- Integration tests become required CI gate for release branches.

### Phase D (Readiness: 2-3 weeks) — Operations & Security Automation

Goal: improve production confidence and incident recoverability.

Actions:
1. Introduce SLO-focused telemetry (deployment latency/error rates, reconnect rates).
2. Add automated policy checks for security posture assumptions.
3. Finalize operator runbooks for failed deploys, stuck jobs, and agent unavailability.

Exit criteria:
- SLO dashboards and runbooks are available and referenced by release process.

## Practical Next 5 Tasks

1. Add and commit Go dependency lock state (`go.sum`) and resolve imports.
2. Standardize proto generation path and commit generated artifacts.
3. Implement `make verify` and wire into CI.
4. Update docs with clean-checkout workflow.
5. Open a follow-up issue for Rust warning cleanup (non-blocking debt).
