# P1 installation, onboarding, MFA, and upgrade implementation

This document explains the remaining P1 implementation. The root changelog keeps
only a condensed Unreleased entry for the next owner-approved version bump.
No version tag, remote release, commit, or push is part of this work.

## Boundaries and decisions

- Support Ubuntu 24.04 with rootful Docker, Nginx, and a native systemd agent.
- Use one concrete administrator CLI and the existing Compose/migration authority.
- Pin `github.com/pquerna/otp` at v1.5.0, including its locked barcode dependency.
  Authenticator parsing/generation uses that implementation; no new MFA framework.
- Encrypt authenticator secrets with the existing account-bound AES-GCM service.
- Keep a pending second-factor challenge separate from a usable session. Its
  opaque value is carried only in an HttpOnly cookie and stored only as a hash.
- Require MFA for both password and magic-link login once enrolled. Password reset
  preserves the factor; enrollment and local recovery invalidate prior sessions.
- Bound challenges to five minutes/five attempts, enrollment to ten minutes, and
  codes to single use. Preserve MFA proof during ordinary session refresh.
- Generate independent secrets into private files. Setup keys are never API logs.
- Package checksummed, versioned binaries/images with their schema/source identity.
  Source changes during a release build abort manifest creation.
- Use an explicit local HTTP/2 authority for the Go-to-agent Unix connection;
  the socket pathname is not an HTTP host. Kernel peer-credential checks and
  privileged RPC capabilities remain unchanged.
- Use the IPv4 listener address in the frontend probe; Alpine's localhost
  resolution can select IPv6 when Nginx is listening only on IPv4.
- Validate migration history before starting or upgrading. Existing-schema
  migration requires a real database backup whose checksum matches its proof.
- Quiesce management for upgrades, retain the old release, and restore executable,
  database, keys, and configuration together. Never run an old API against a newer
  unknown schema or silently retry destructive restoration.
- Lifecycle completion checks the API, frontend proxy, and local HTTPS entry point
  together, using the configured certificate and hostname. API readiness alone
  must not report successful recovery while the browser entry point is unavailable.
- Restore deliberately revokes sessions, email capabilities, and saved recovery
  codes, rotates the JWT signing secret, and advances the OTP replay boundary.
  Rolling data back must not resurrect capabilities revoked after the snapshot.

## Files changed and reasoning

| Path | Change and reason |
| --- | --- |
| `.env.example` | Adds the private bootstrap-key setting; no live credentials. |
| `README.md` | Documents private initialization, supported installation, MFA, and release commands; retains the approved documentation moves. |
| `CHANGELOG.md` | Adds concise Unreleased highlights without inventing a published version. |
| `go.mod` | Pins the approved OTP dependency. |
| `go.sum` | Records only its required module checksums. |
| `toolchain.env` | Pins the Rust release builder digest and protoc 21.12 Linux ARM64 checksum; existing language versions remain unchanged. |
| `api/Dockerfile` | Includes the administrator/migration utility in the API image for controlled local operations. |
| `api/cmd/kari-admin/main.go` | New concrete CLI for private initialization, bundle metadata, installation, lifecycle, backup, upgrade, restore, and emergency recovery. |
| `api/internal/operations/host.go` | Implements supported-host checks, private state, artifact validation, discrete process arguments, service installation, and matching-state recovery. |
| `api/cmd/kari-api/main.go` | Rejects incompatible schemas, stops logging setup keys, and wires MFA/onboarding to actual dependencies. |
| `api/cmd/migrate/main.go` | Adds a non-mutating compatibility check and requires backup proof before advancing existing schemas. |
| `api/internal/config/config.go` | Loads private setup-key and release identity settings explicitly. |
| `api/internal/core/domain/identity.go` | Represents pending/verified MFA state and the existing challenge cooldown. |
| `api/internal/core/services/mfa.go` | New enrollment, encrypted-secret verification, recovery-code generation, and second-factor completion logic. |
| `api/internal/core/services/auth_service.go` | Issues challenges instead of sessions when MFA is enabled, and retains verified-factor state on refresh. |
| `api/internal/core/services/email_access.go` | Uses the same MFA-aware login preparation for magic links. |
| `api/internal/core/services/token_service.go` | Carries verified-factor identity in signed access claims. |
| `api/internal/db/postgres/user_repo.go` | Scans MFA state, prevents pending sessions from authenticating/refreshing, and atomically applies magic-link grants. |
| `api/internal/db/postgres/mfa_repo.go` | New transactional storage for enrollment, bounded attempts, anti-replay, recovery-code consumption, and local administrator recovery. |
| `api/internal/db/migrations/000007_mfa.sql` | Forward-only MFA schema. Previously applied migrations are untouched. |
| `api/internal/db/migrations/manifest.txt` | Appends the new canonical migration. |
| `api/internal/db/migrator.go` | Rejects unknown/modified history and validates database-backup proof. |
| `api/internal/db/migrator_test.go` | Updates the expected canonical head without weakening assertions. |
| `api/internal/db/contract/repository_contract_test.go` | Updates the real-database schema-head contract. |
| `api/internal/db/contract/auth_journey_test.go` | Adds the concrete MFA/recovery journey and actual unbacked/backed schema-upgrade checks. |
| `api/internal/api/handlers/auth.go` | Exchanges the first factor for a private challenge, handles MFA actions, and exposes production policy accurately. |
| `api/internal/api/handlers/email_access.go` | Prompts for MFA after magic links and preserves policy in returned account data. |
| `api/internal/api/handlers/setup_handler.go` | Reports actual onboarding dependencies and private setup-key guidance. |
| `api/internal/api/router/router.go` | Wires account-security routes and blocks production workload management without a verified second factor. |
| `docker-compose.yml` | Supports versioned images/private settings and lets the frontend display unavailable dependency diagnostics. |
| `deploy/systemd/kari-agent.service` | Gives the socket parent the API group so the unprivileged API can traverse it. |
| `deploy/systemd/kari-panel.service` | New boot lifecycle for the container topology. |
| `deploy/systemd/agent.env.example` | Corrects the proxy base directory expected by the existing agent. |
| `frontend/src/lib/api.ts` | Types factor policy and avoids treating a bad MFA code as a refreshable access-token failure. |
| `frontend/src/App.tsx` | Routes unenrolled production users to account setup and propagates account security changes. |
| `frontend/src/pages/AccountPage.tsx` | Displays actual onboarding checks, authenticator enrollment, and one-time recovery-code download. |
| `frontend/src/pages/LoginPage.tsx` | Completes the challenge with an authenticator/recovery code before displaying authenticated UI. |
| `frontend/src/pages/EmailLinkPage.tsx` | Continues magic links into the same second-factor screen. |
| `frontend/src/pages/SetupPage.tsx` | Uses private operator-key instructions instead of log disclosure. |
| `frontend/tests/auth.spec.ts` | Adds one disposable-stack MFA journey for password and magic-link UI paths. |
| `scripts/build-release.sh` | Builds local immutable bundles from pinned tools, with source-tree correlation and no publishing. |
| `scripts/install-protoc.sh` | Supports the existing protoc version on Linux ARM64 with a verified checksum. |
| `scripts/topology-smoke.sh` | Supplies a disposable private setup key without extracting secrets from logs. |
| `docs/OPERATIONS.md` | Documents prerequisites, installation, MFA, root recovery, and guarded upgrade/restoration boundaries. |
| `docs/TECHNICAL_SPEC.md` | Replaces stale setup/recovery descriptions with implemented boundaries. |
| `docs/P1_IMPLEMENTATION_SUMMARY.md` | This separate explanation and file inventory. |

## Previously requested layout changes retained

`DEVELOPMENT.md` moves to `docs/DEVELOPMENT.md`, README links follow it, and
`scripts/verify-docs.sh` resolves links relative to each document. `CHANGELOG.md`,
README, CONTRIBUTING, SECURITY, and AGENTS remain at the repository root.

## Verification scope and remaining release gates

Focused account checks cover enrollment, first-factor isolation, single-use codes,
refresh, magic links, password reset, and root recovery. A separate owned database
fixture applies the unchanged earlier migrations, rejects an unbacked upgrade,
then advances with an actual pg_dump archive. The host rehearsal uses an isolated
Ubuntu 24.04 ARM64 VM passed installation, version upgrade, tampered-artifact and
downgrade refusal, local root recovery, and matching-state restoration. A focused
restore check also confirmed that revoked refresh sessions and saved recovery
codes stay invalid after rollback while authenticator login and web readiness
work. The real-database contracts and the single MFA browser journey passed.
The full verification suite and GitHub CI were not rerun for this change.

SMTP inbox delivery and public-CA issuance require operator configuration. The
installer accepts a provided valid hostname certificate; it does not operate an
ACME service. The same-host rollback procedure does not certify replacement-host
disaster recovery or customer workload backups. Tenant isolation, deployment-state
correctness, and privileged-agent RPC hardening remain P0 gates. P1 completion
alone does not make Karı production-ready.
