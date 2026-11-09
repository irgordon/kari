# P2: first static website workflow

This records the first increment at `ba97e25`. See
[P2 completion](P2_COMPLETION_SUMMARY.md) for the subsequent Node, database,
files/SFTP, backup/restore, and server-operations implementation. The limitations
below describe this earlier increment rather than the current feature set.

This increment connects website creation, publication, domain routing, TLS, and
saved dashboard state. It is deliberately limited to prepared static content from
public HTTPS Git repositories. It does not complete P2 or certify production use.
The root changelog contains condensed Unreleased entries for the next version
bump; this document records implementation choices and file-level reasoning.

## Decisions

- Reuse the existing domain/application/deployment tables and canonical migrations.
  Creating a website commits its ownership and domain binding in one transaction.
- Keep a concrete `Sites` service and HTTP handler. Do not activate the old
  generic application/domain APIs merely to preserve their incompatible UI.
- Queue real application IDs, reject overlapping site operations, scope result
  reads by owner, and require existing application/domain permissions and MFA.
- Use three narrow native operations for publication, ACME challenge files, and
  certificate activation. The broader legacy RPC surface remains a P0 gate.
- Fetch as a separate site identity with a bounded systemd service, fixed Git
  arguments, disabled hooks, and no repository code execution. Copy ordinary,
  nonhidden files into root-managed releases; switch the current release only
  after Nginx accepts the candidate configuration.
- Require explicit agent success rather than interpreting stream EOF or log text
  as publication success. Viewing logs does not control the worker lifetime.
- Reconcile interrupted operations conservatively as failed/uncertain on startup,
  allowing an operator to inspect and explicitly republish. Support one API instance.
- Reuse pinned Lego for HTTP-01. Do not use unrestricted file writes or package
  commands to create/remove challenges. Require explicit subscriber consent and
  verified account email. Supplied certificates also support operator-managed TLS.
- Preserve previous proxy/certificate configuration on failure. Wait for the
  expected served certificate before reporting successful TLS activation.
- Report saved hosting data and errors accurately; remove zero-valued fake
  metrics and the unsupported blanket “All systems operational” message.

## Files changed and reasoning

| Path | State | Reason |
| --- | --- | --- |
| `proto/kari/agent/v1/agent.proto` | Modified | Defines the three site-specific host operations. |
| `api/internal/grpc/rustagent/agent.pb.go` | Generated | Pinned generator output for the new messages. |
| `api/internal/grpc/rustagent/agent_grpc.pb.go` | Generated | Pinned client/server RPC definitions. |
| `agent/src/proto/agent_descriptor.bin` | Generated | Matching Rust descriptor. |
| `agent/src/sys/hosting.rs` | New | Implements confined static publication, configuration rollback, challenge files, and TLS readiness; includes one filesystem boundary test. |
| `agent/src/sys/mod.rs` | Modified | Registers the concrete hosting module. |
| `agent/src/server.rs` | Modified | Serializes the new privileged operations and returns explicit gRPC failures. |
| `api/internal/core/services/sites.go` | New | Owns transactional site creation, authorization-scoped queries, queueing, interruption recovery, and certificate orchestration. |
| `api/internal/api/handlers/sites.go` | New | Parses requests, applies deadlines and consent, and returns bounded, safe errors. |
| `api/internal/adapters/acme_provider.go` | Modified | Replaces the unused arbitrary-path challenge implementation with site-bound RPCs; checks agent results and preserves cancellation. |
| `api/internal/api/router/router.go` | Modified | Mounts permission-protected site routes; allows bounded certificate issuance time. |
| `api/cmd/kari-api/main.go` | Modified | Constructs the concrete hosting path and reconciles interrupted operations before starting the worker. |
| `api/internal/worker/deployment_worker.go` | Modified | Uses explicit static publication acknowledgements and removes unused SSH/cancellation dependencies from this worker. |
| `api/internal/db/postgres/deployment_repository.go` | Modified | Commits publication and related saved site/domain state together, preserving a previous successful site after replacement failure. |
| `api/internal/db/contract/sites_test.go` | New | Exercises real PostgreSQL ownership, duplicate creation, queueing, final state, logs, and interruption handling. |
| `api/internal/operations/host.go` | Modified | Checks Git/OpenSSL/systemd-run prerequisites and permits bounded ACME response time through panel Nginx. |
| `docker-compose.yml` | Modified | Passes the optional ACME directory setting to the API. |
| `frontend/nginx.conf` | Modified | Keeps bounded certificate requests open through the frontend proxy. |
| `frontend/src/pages/SitesPage.tsx` | New | Provides site creation, publication, persisted results, DNS guidance, and ACME/import TLS controls. |
| `frontend/src/pages/DashboardPage.tsx` | Modified | Shows real saved site/certificate state and explicit unavailable/error states. |
| `frontend/src/App.tsx` | Modified | Connects Websites navigation and redirects obsolete domain/deployment pages. |
| `README.md` | Modified | Defines the actual supported hosting increment and links operations. |
| `docs/OPERATIONS.md` | Modified | Documents inputs, limits, DNS/TLS, manual renewal, and remaining workload operations. |
| `docs/TECHNICAL_SPEC.md` | Modified | Describes the new supported host boundary without claiming broad RPC safety. |
| `CHANGELOG.md` | Modified | Condensed next-version highlights. |
| `docs/P2_IMPLEMENTATION_SUMMARY.md` | New | This detailed file inventory and reasoning. |

## Focused verification

- Affected Go package tests and vet passed. Fresh PostgreSQL site/deployment
  contracts passed with race detection and uncached results.
- Strict Rust Clippy and the single static-file boundary test passed. Protobuf
  output matched the existing pinned generators; library/toolchain pins stayed fixed.
- Frontend lint, type checking, and production build passed. The first lint run
  caught a render-time clock read; it was moved into a lazy state initializer.
- A disposable Ubuntu 24.04 ARM64 host used real PostgreSQL, API, frontend,
  native agent, systemd, and Nginx. Public Git publication worked; a nonexistent
  replacement branch failed while the prior website stayed available.
- The first immediate TLS check exposed asynchronous Nginx reload completion.
  The correction waits for the expected served certificate. The focused rerun
  passed certificate activation, invalid-key rejection, and HTTPS republishing.
- One pinned-Playwright browser check created and published a site through the
  real UI, reloaded its saved result, opened TLS controls, and checked the dashboard.
  Its loopback panel used a disposable self-signed certificate. Host API/website
  HTTPS checks used explicit certificate trust and hostname verification.

Unpublished local candidates were rehearsal artifacts, not public versions or
releases. No live ACME issuance, public DNS validation, SMTP delivery, or x86-64
host rehearsal was performed. ACME orchestration compiled, but public issuance
remains an operator acceptance check. Full project suites and GitHub CI were not
rerun. No commit, push, tag, or repository-setting change is included.

## Remaining P2 and release boundaries

Next deliver customer database provisioning, files/SFTP, manual workload backup
and restore, application runtimes, and broader server operations/live telemetry.
Certificate renewal, release retention, and disk use currently require operator
attention. Control-plane backup does not capture website directories. Foundational
tenant isolation, trustworthy general deployment semantics, and privileged-agent
RPC hardening remain separate production blockers; this narrower workflow does
not resolve or certify those entire surfaces.
