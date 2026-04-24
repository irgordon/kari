# Project State Review & Phased Recommendations (2026-04-24)

## Executive Summary

Karı has a **strong architectural foundation** (clear separation between UI/Go API/Rust agent, security-oriented design docs, and container orchestration), but the repository is currently in a **partially integrated state** where key build/test workflows do not pass end-to-end.

Most critically, the Go backend dependency/protobuf setup is incomplete in this checkout, which currently blocks full backend compilation and any integrated validation pipeline.

---

## Current State Snapshot

### 1) Architecture and repo organization: strong
- The codebase follows a clear 3-layer model (Window/Brain/Muscle) with detailed architecture and security documents.
- The monorepo structure and docker-compose topology are coherent and production-minded (internal backplane, service healthchecks, explicit role separation).

### 2) Rust agent: healthy and testable
- `cargo test` in `agent/` passes (18 passing tests), indicating the agent module is in relatively good shape and has meaningful test coverage for input hardening and security-sensitive logic.
- There are compile warnings (unused imports/traits/fields), signaling low immediate risk but some maintainability debt.

### 3) Go API: blocked at dependency/proto integration
- `go test ./...` fails broadly due to missing required modules and unresolved internal proto/grpc package paths.
- Repository root `go.mod` exists, but there is no dependency list/go.sum in this checkout, so the backend cannot compile reproducibly as-is.

### 4) Frontend: likely functional code, but local test workflow incomplete
- Frontend has proper scripts (`test`, `check`, `lint`) in `frontend/package.json`.
- `npm --prefix frontend run test` fails here because `vitest` is unavailable in the current environment (dependencies not installed in `frontend/node_modules` for this run).

### 5) Operational workflow maturity: mixed
- Docker + Make targets show strong operational intent.
- However, “default local validation” is not yet turnkey because backend compile/test cannot run from a clean checkout without dependency/proto remediation.

---

## Key Risks to Delivery

1. **Build reproducibility risk (Critical):** Backend not compiling blocks CI confidence and releases.
2. **Integration risk (High):** gRPC/protobuf contract paths appear inconsistent between code and generated artifacts.
3. **Velocity risk (Medium):** Developers cannot rely on one-shot checks from a fresh clone.
4. **Quality drift risk (Medium):** Rust warnings and duplicate/legacy paths in API packages may accumulate maintenance overhead.

---

## Phased Plan (Recommended)

## Phase 0 (0-2 days): Restore baseline build integrity
**Goal:** Make fresh-checkout backend compile and tests runnable.

Actions:
1. Reconstruct/normalize Go dependency state:
   - Ensure canonical module root strategy (single root `go.mod` or scoped `api/go.mod`, but not ambiguous hybrid).
   - Run dependency resolution and commit lock artifacts (`go.sum`).
2. Fix protobuf generation/import contract:
   - Regenerate stubs via a single documented command.
   - Align all imports (e.g., `kari/api/proto/...`) to generated output paths.
3. Add a quick “bootstrap verify” target:
   - e.g., `make verify` runs backend compile + agent tests + frontend install/check gates.

Exit criteria:
- `go test ./...` passes (or clearly scoped skips).
- Protobuf generation is deterministic and documented.

## Phase 1 (2-5 days): Establish reliable developer and CI gates
**Goal:** Ensure every PR can be validated consistently.

Actions:
1. Add CI jobs for:
   - Go test/build
   - Rust fmt/clippy/test
   - Frontend install/check/test
2. Add “from scratch” setup path:
   - One command/script that installs deps and runs all checks.
3. Treat warnings intentionally:
   - Decide warning policy (allowlist vs fail-on-warning for specific modules).

Exit criteria:
- Green CI on all 3 layers.
- New contributor can run a documented single-command validation flow.

## Phase 2 (1-2 weeks): Integration hardening and reliability
**Goal:** Reduce runtime and concurrency failure modes in core deployment flow.

Actions:
1. Address known concurrency/reliability issues already identified in `AUDIT.md`:
   - Timestamp collision risk in deployment directory naming.
   - Single deployment worker bottleneck.
   - Unbounded task spawning risk in agent.
2. Add integration tests:
   - API↔Agent gRPC over UDS happy path and failure modes.
   - Deployment pipeline rollback/cleanup behavior.
3. Add structured rollback semantics:
   - Ensure partial deployment artifacts are cleaned automatically on failure.

Exit criteria:
- Concurrency fixes merged and covered by tests.
- Integration tests validate at least one end-to-end deployment simulation.

## Phase 3 (2-4 weeks): Production readiness and observability uplift
**Goal:** Move from “works locally” to “operationally resilient”.

Actions:
1. SLO-oriented telemetry:
   - Deployment latency/error rate metrics.
   - gRPC link health and reconnect metrics.
2. Security posture automation:
   - CI audit checks for image hardening assumptions and secret-handling invariants.
3. Runbook completion:
   - Incident playbooks for failed deployments, stuck workers, and agent unavailability.

Exit criteria:
- Defined operational SLOs and dashboards.
- Documented, testable recovery workflows.

---

## Suggested Prioritization Matrix

- **Do now:** Backend dependency/proto repair, deterministic builds.
- **Do next:** CI gates and integration tests.
- **Then:** Concurrency throughput and rollback hardening.
- **Later:** Advanced observability/security automation and scaling improvements.

---

## Practical Next 3 Tasks

1. Create/confirm canonical Go module layout and regenerate `go.sum`.
2. Run and commit protobuf generation outputs with standardized import paths.
3. Add a root `make verify` target and wire it into CI.

These three steps will immediately convert Karı from “architecturally strong but integration-fragile” to a project with a stable engineering baseline.
