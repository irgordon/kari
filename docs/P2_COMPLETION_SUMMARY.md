# P2 single-server workload implementation

The remaining approved P2 scope is implemented locally: one Node.js stack,
customer PostgreSQL instances, site data and key-only SFTP, manual workload
backup/restore, and actual server measurements/allowed operations. This builds on
the static hosting increment at `ba97e25`. It does not declare a production release
or certify the legacy privileged RPC surface and broader tenant model.

The root changelog contains condensed Unreleased entries for the next version
bump. This document records the file inventory, reasoning, and validation limits.
No commit, push, Git tag, public release, or repository-setting change is included.

## Implementation choices

- Keep concrete Go services, direct SQL, and explicit native operations. No new
  library, dependency upgrade, framework, generic task system, or plugin layer.
- Support Node.js 24.19.0/npm 11.17.0 using the existing immutable Node image.
  Fetch under a dedicated site identity; run fixed npm steps in bounded,
  unprivileged containers. Published source is read-only, data is persistent,
  and the application must serve `/` on port 3000 before activation.
- Keep the previous application until Nginx acknowledges the new route. A
  loopback-only HTTP probe confirms the routing revision, and TLS activation
  verifies the served certificate. A reload signal alone is not completion.
- Use one PostgreSQL instance/network per customer site, with no published
  database port and a non-superuser application role. Credentials remain in
  agent-private files and are injected as `DATABASE_URL`; backup results never
  return passwords. Restore generates a fresh database credential.
- Reserve the site's application row during mutations and reject overlapping
  deployment, TLS, backup, file, and runtime work. Interrupted acknowledgements
  remain failures/uncertainties rather than invented success. Support one API.
- Apply owner-scoped lookups, existing application/domain permissions, production
  MFA, explicit destructive permission, and typed hostname confirmation before
  invoking the agent. The existing Unix-socket peer gate is unchanged.
- Keep browser file access within site data with descriptor-relative no-follow
  operations. Bound payloads/listing sizes. SFTP uses validated public keys,
  root-owned chroots, internal-sftp, and no password, shell, or forwarding access.
- Quiesce the application and SFTP during snapshots. Save actual database dumps,
  data/release files, active/pending environment, source metadata, and certificates.
  Validate checksums before replacement. Failure during replacement leaves the
  site stopped for repair; uncertain acknowledgement requires host inspection.
- Preserve current SFTP authorization on restore instead of reviving revoked
  keys. Keep application/custom secrets out of response metadata and logs.
- Expose actual host CPU/memory/root-disk/uptime and fixed service states on
  refresh. Allow validated Nginx reload; keep arbitrary host command execution,
  package operations, Docker/SSH/agent lifecycle, and reboot out of these controls.

## Files changed and reasoning

| Path | State | Purpose |
| --- | --- | --- |
| `proto/kari/agent/v1/agent.proto` | Modified | Defines the seven approved typed workload methods, public result metadata, and actual host measurements. |
| `api/internal/grpc/rustagent/agent.pb.go` | Generated | Pinned Go message output. |
| `api/internal/grpc/rustagent/agent_grpc.pb.go` | Generated | Pinned Go RPC output. |
| `agent/src/proto/agent_descriptor.bin` | Generated | Corresponding Rust descriptor. |
| `agent/src/config.rs` | Modified | Accepts digest-pinned workload image settings supplied by the verified release. |
| `agent/src/server.rs` | Modified | Wires the specifically approved methods, serializes host mutation, and reports memory/disk measurements. |
| `agent/src/sys/mod.rs` | Modified | Registers the three concrete workload modules. |
| `agent/src/sys/hosting.rs` | Modified | Shares actual site preparation/fetch, adds static data routing/source metadata, and waits for acknowledged proxy routing. |
| `agent/src/sys/workloads.rs` | New | Implements constrained Node builds/lifecycle, bounded stdout/stderr, separate databases, public-key SFTP configuration, and fixed service operations. |
| `agent/src/sys/site_files.rs` | New | Implements descriptor-relative file operations and a focused link/parent containment test. |
| `agent/src/sys/site_backups.rs` | New | Implements bounded, private snapshots, actual dumps, checksum validation, replacement, fresh database credentials, and the saved-release link boundary test. |
| `api/internal/core/services/sites.go` | Modified | Persists runtime choice, exposes owner-scoped detail, queues the correct runtime, and respects reservations. |
| `api/internal/core/services/workloads.go` | New | Validates workload inputs/confirmation, reserves mutations, checks ownership, and reconciles database/restored metadata. |
| `api/internal/api/handlers/sites.go` | Modified | Accepts runtime choice and initializes the saved website on the native host. |
| `api/internal/api/handlers/workloads.go` | New | Separates safe reads from mutations, enforces destructive permission, and exposes implemented system controls with safe errors. |
| `api/internal/api/router/router.go` | Modified | Mounts site detail/workload and server routes with existing permissions and MFA enforcement. |
| `api/cmd/kari-api/main.go` | Modified | Constructs the concrete workload handler. |
| `api/internal/worker/deployment_worker.go` | Modified | Selects static/Node publication and bounds the complete Node operation. |
| `api/internal/db/postgres/deployment_repository.go` | Modified | Updates Node/static final state while preserving a prior application's state on failed replacement. |
| `api/internal/db/migrations/000008_workloads.sql` | New | Forward migration for operation reservations and nonsecret customer database metadata. |
| `api/internal/db/migrations/manifest.txt` | Modified | Appends migration 8; migrations 1–7 are unchanged. |
| `api/internal/db/migrator_test.go` | Modified | Advances the asserted canonical migration count. |
| `api/internal/db/contract/repository_contract_test.go` | Modified | Advances the real-database schema-head assertion. |
| `api/internal/db/contract/auth_journey_test.go` | Modified | Advances the expected head of the existing backed-upgrade check. |
| `api/internal/db/contract/sites_test.go` | Modified | Covers Node persistence, cross-owner denial, confirmation, key options, and deployment/workload exclusion against real PostgreSQL. |
| `api/internal/operations/host.go` | Modified | Checks SSH/checksum/process prerequisites and installs workload image settings from verified toolchain metadata. |
| `scripts/build-release.sh` | Modified | Includes the existing pinned Node image in local bundles. |
| `deploy/systemd/agent.env.example` | Modified | Explains that workload image settings come from the verified release, without duplicating floating image authority. |
| `frontend/src/pages/WebsitePage.tsx` | New | Implements runtime/environment, database, files, SFTP, and backup/restore controls with explicit feedback and confirmation. |
| `frontend/src/pages/SitesPage.tsx` | Modified | Adds runtime selection and management navigation. |
| `frontend/src/pages/SystemPage.tsx` | Modified | Replaces unwired metrics/log claims with real measurements, fixed service state, and validated reload. |
| `frontend/src/App.tsx` | Modified | Mounts management pages and redirects unsupported global settings. |
| `README.md` | Modified | Describes the implemented stack and prerequisites. |
| `docs/OPERATIONS.md` | Modified | Documents the actual customer workflows, limits, recovery behavior, and production boundaries. |
| `docs/TECHNICAL_SPEC.md` | Modified | Updates runtime, routing, workload, and recovery responsibilities. |
| `docs/P2_IMPLEMENTATION_SUMMARY.md` | Modified | Identifies the earlier static-only report as historical and links this current implementation. |
| `CHANGELOG.md` | Modified | Condensed next-version entry. |
| `docs/P2_COMPLETION_SUMMARY.md` | New | This separate detailed implementation record. |

## Focused validation and observed corrections

| Check | Result |
| --- | --- |
| Affected Go package tests, uncached | Passed. |
| Affected Go vet | Passed. |
| Fresh PostgreSQL migration to head 8 | Passed. |
| Site/affected deployment contracts with `-race -count=1` | Passed. |
| Rust strict Clippy with the existing warning policy | Passed after correcting reported implementation errors. |
| Two focused file/snapshot containment tests | Passed. |
| Pinned protobuf drift verification | Passed. |
| Frontend lint, type checking, production build | Passed. |
| Ubuntu 24.04 ARM64 installation and candidate upgrades | Passed on a disposable VM without host mounts. |
| Actual Node build, environment, persistent data, healthy publication | Passed against a real local HTTPS Git server. |
| Separate database and non-superuser role | Passed against actual PostgreSQL. |
| Actual SFTP transfer, chroot, and key revocation | Passed; revocation did not stop the Node application. |
| Browser backup/change/restore | Passed for data files, SQL contents, environment, release, and TLS. |
| Fresh database credential after restore | Passed; passwords were not emitted in results. |
| Runtime start/stop/restart and stdout/stderr logs | Passed through the real browser/API/agent stack. |
| Destructive restore after removing actor delete permission | Returned 403; customer data was unchanged. |
| Real server metrics and configuration-validated Nginx reload | Passed. |

Failures were kept visible and corrected rather than ignored:

- Initial native compilation exposed a misplaced shared-fetch function boundary
  and an unused import; both were corrected. Strict Clippy later caught a
  needless borrow after snapshot handling changed; it was removed.
- The initial Go compile check caught the generated `SFTPRequest` acronym.
- The local Git fixture omitted Content-Length, causing a TLS transport error;
  only that fixture's framing was corrected. TLS verification stayed enabled.
- The SFTP fixture used `-1` instead of stdin `-` for its batch file; the fixture
  was corrected and the unchanged transfer/confinement assertions passed.
- The browser caught the controlled file textarea changing its implicit label;
  an explicit stable label fixed the actual editor instead of weakening the locator.
- The immediate post-restore HTTP assertion exposed an old Nginx worker routing
  to the retired port. Native activation now waits for the new routing revision;
  the original immediate assertion then passed, including over verified HTTPS.

The host run used actual API/DB/frontend/agent/systemd/Nginx/OpenSSH processes;
none of those boundaries was replaced with a mock. A private local Git fixture
and fake credentials lived only in the disposable environment. The control-panel
browser used its self-signed fixture certificate; customer website HTTPS was
checked with explicit certificate trust and hostname verification.

The seven privileged methods were explicitly approved by the owner after review
identified the earlier RPC deferral. An earlier password-returning backup draft
was rejected and removed. The implemented restore keeps credentials in the agent.

## Limits and remaining release work

This completes the approved single-server P2 feature scope, not cPanel parity or
a production-readiness certification. Node is the only application runtime;
other stacks, private Git credentials, native dependency install hooks, and
arbitrary runtime images/host commands remain unsupported. File/snapshot limits
and same-host recovery boundaries are specified in operations.

No live public ACME issuance/DNS validation, real SMTP delivery, x86-64 host
rehearsal, replacement-host restore, or full privileged-RPC security audit was
performed. Public ACME acceptance remains operator work. The full verification
suite and GitHub CI were not rerun; focused checks and the actual host journey
are the validation claimed here. Application libraries and toolchain/image pins
were not upgraded.

P0 isolation/general deployment/RPC release gates remain. Multi-customer roles,
quotas, hosted mail, and commercial shared-hosting workflows remain P3. Automated
off-host backup policies, retention, renewal automation, expanded diagnostics,
and broader release polish remain P4. Control-plane and workload backups must
both be planned; neither alone supplies complete replacement-host disaster recovery.
