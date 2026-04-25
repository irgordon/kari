# Project State Review & Validation (2026-04-25)

## Scope
This review validates the current repository state against the phase plan (A-D), with emphasis on the two critical blockers:

1. Go module + proto import mismatch.
2. Frontend dependency bootstrap on clean checkout.

## Validation Checks Run

1. `go test ./...`
2. `make dev`
3. Static inspection of `go.mod`, `Makefile`, and gRPC/proto paths.

## Current State vs Plan

### Phase A — Deterministic Backend Recovery (**still blocked**)

#### A1 — Normalize Go module state: **❌ blocked**

Observed on April 25, 2026:
- `go test ./...` fails due to missing external deps from `go.mod` / `go.sum`.
- Internal imports still reference `kari/...` while module is `github.com/irgordon/kari`.
- Import `kari/api/internal/grpc/rustagent` fails and generated Go stubs directory is absent.

This confirms the primary blocker remains unresolved.

#### A2 — Proto generation correctness: **🔶 mostly complete, but operationally blocked**

- Scripts include module-aware generation logic and drift guard.
- In this environment, generation is skipped unless `protoc-gen-go` and `protoc-gen-go-grpc` are installed.
- Go stubs required by backend are not currently present in-tree.

#### A3 — Commit generated proto artifacts: **❌ blocked on A1/A2 execution**

- Rust descriptor artifact path is wired in script.
- Go generated artifacts are not currently committed under `api/internal/grpc/rustagent`.

---

### Phase B — Unified Developer Validation Path (**improving**)

#### B1 — Single entrypoint (`make dev`): **✔️ now implemented in this review pass**

Changes made during this review:
- Added `frontend-setup` target using deterministic install:
  - `npm --prefix frontend ci`
- Added `dev` target:
  - `dev: frontend-setup verify`
- Removed duplicate frontend install from `verify` so bootstrap is centralized in `frontend-setup`.

#### B2 — CI alignment: **🔶 partial**

- CI already runs `make dev`, but this previously failed immediately because `dev` target did not exist.
- With `dev` target now present, CI progresses further, but remains blocked by backend module/import failures (Phase A).

#### B3 — Frontend reproducibility: **✔️ addressed in Makefile**

- Clean-checkout frontend dependency bootstrap is now explicit and mandatory via `frontend-setup`.
- This resolves the second critical blocker at workflow level.

---

### Phase C — Integration Reliability Hardening: **❌ not started**

No integration test harness for API ↔ Agent deterministic behavior was identified in this pass.

### Phase D — Operational Readiness: **❌ not started**

No additional telemetry/SLO automation or runbook finalization changes were identified in this pass.

## Priority-Ordered Next Actions

1. **Fix Go module/import normalization (A1)**
   - Choose one canonical import root and rewrite all internal Go imports to match `go.mod`.
   - Populate `go.mod`/`go.sum` with required dependencies.
2. **Generate and commit Go gRPC stubs (A3)**
   - Ensure `api/internal/grpc/rustagent/*.go` exists and matches `go_package` + module path.
3. **Re-run full deterministic path**
   - `make dev`
   - CI verify workflow should then become meaningful for regression prevention.

## Conclusion

- **Blocker #2 (frontend bootstrap)** is now fixed in the repository workflow by introducing `frontend-setup` and wiring `dev` through it.
- **Blocker #1 (Go module + proto imports)** remains the primary production blocker and must be resolved next before Phase C/D work is worthwhile.
