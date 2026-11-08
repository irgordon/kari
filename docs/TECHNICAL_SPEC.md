# Karı Technical Specification

## 1. Overview

Karı is a server control panel with a React frontend, an unprivileged Go API,
a privileged Rust host agent, and PostgreSQL. Its intended responsibilities
include application deployment, domains, proxy configuration, certificates,
and host service management.

This specification describes system responsibilities and required boundaries.
It does not certify every existing implementation as production-ready.
[AGENTS.md](../AGENTS.md) defines implementation practices;
[operations](OPERATIONS.md) defines installation and runtime procedures.
The earlier [archived specification](archive/TECHNICAL_SPEC.md) is historical.

## 2. System Architecture

The supported topology is PostgreSQL, the Go API and migration service, and the
React frontend in containers, with the Rust agent installed natively on a Linux
systemd host. The agent is not an ordinary containerized application.

- **React frontend:** A Vite-built SPA served by Nginx. Calls the Go REST API
  through `/api/`. The browser sends HttpOnly cookies; JavaScript cannot read or
  directly manage their contents.
- **Go API:** Runs without root privileges. Handles HTTP requests, authentication,
  authorization, database access, and orchestration of agent operations.
- **Rust agent:** Runs as root and performs approved host operations through gRPC
  over a local Unix socket. The API must not gain general root execution rights.
- **PostgreSQL:** Stores application state, authorization data, setup completion,
  and sessions. Migrations run before the API starts.

`docker-compose.yml` defines the container topology;
`deploy/systemd/kari-agent.service` defines the native agent service. The shared
socket is `/run/kari/agent.sock`. Socket ownership, permissions, and peer-identity
checks must restrict access to the intended API identity and fail closed.

Linux and systemd are the supported host boundary. Configurable paths and users
do not establish support for FreeBSD or every Linux distribution.

## 3. Core Design Principles

### Simple, Explicit Boundaries

Karı favors direct, explicit implementations over abstraction-heavy
architecture.

Interfaces and traits are used only where they provide a concrete benefit,
such as:

- separating privileged and unprivileged components
- supporting multiple real implementations
- isolating an external system dependency
- enabling a security-relevant boundary

Do not introduce interfaces, traits, adapters, factories, providers, or
service layers solely for architectural symmetry or hypothetical future
requirements.

The preferred implementation is the simplest design that preserves Karı's
security boundaries and remains easy to understand and test.

### Input, Execution, and Resource Boundaries

Validate externally controlled values before privileged operations. Construct
processes with discrete arguments, not shell command strings assembled from input.
Validate option-like arguments, paths, domains, ports, and execution identities
according to the operation being performed.

Bound externally influenced memory use, tasks, streams, filesystem traversal,
and query results. A bounded channel alone does not prove that a daemon cannot
exhaust memory. Configuration defaults must not be described as platform independence.

These are requirements for implementation and review, not a claim that the
existing privileged RPC surface has completed hardening.

## 4. PostgreSQL Database Contract

The ordered migration authority is
`api/internal/db/migrations/manifest.txt`. The migration runner applies pending
changes transactionally and records versions and SHA-256 checksums in
`schema_migrations`. Preserve applied history and add narrowly scoped forward
migrations. Published legacy SQL under `api/internal/db/legacy/v1.0.0/` is not an
automatic upgrade path.

The schema includes users, roles, permissions, role assignments, installation
completion, sessions, domains, applications, deployments, deployment logs,
system alerts, and system profiles. Use the migration files for exact column,
constraint, index, and status definitions; do not maintain a competing schema here.

Prefer relational columns and explicit SQL. Use JSONB where flexible data is
justified. Add indexes for concrete queries or constraints. UUIDs do not replace
ownership checks or authorization. Bound list queries; do not claim universal
pagination limits or sub-millisecond performance without evidence.

## 5. Application Runtimes and Host Isolation

Application processes must run under their intended unprivileged host identity.
Restrict application filesystem access and scope destructive operations to
approved paths. Use systemd service restrictions appropriate to the operation.
Do not equate a generated unit or Unix permissions with container-level isolation.

Host operations reside in `agent/src/sys/`. The native agent and application
units have different responsibilities; the agent's privileges do not belong to
application build or runtime processes.

## 6. Deployment and Log Streaming

Deployment operations must validate repository and execution inputs, isolate
untrusted builds from root privileges, preserve active releases during cleanup,
and report failures and terminal outcomes accurately. Disable repository hooks
where untrusted repositories are cloned. Bound log transport and make any loss
of log data explicit.

Deployment state correctness, recovery, and log streaming remain recovery work.
Do not describe the current implementation as guaranteeing zero-downtime swaps,
safe rollback, or lossless logs solely because those behaviors were intended in
the earlier design.

## 7. Network and Routing

The frontend Nginx container serves static assets and proxies `/api/` to the Go
API. Host application proxy configuration is a separate privileged operation.
Validate hostnames and generated configuration before activation, preserve valid
configuration on failure, and report validation and reload errors.

Production requires HTTPS at the trusted frontend reverse proxy, preservation
of the original Host header, and explicitly configured allowed origins. Host
service reloads use the supported Linux/systemd boundary; `systemctl` is not a
platform-independent service manager.

## 8. Certificates and Secrets

Certificate operations must protect private keys during generation, Unix-socket
transfer, storage, and renewal. Create secret files with restrictive permissions
and constrain their paths. Do not expose secrets through errors, telemetry, or
ordinary application logs.

Avoid unnecessary plaintext copies and clear sensitive buffers where practical.
Do not promise CPU-register-only storage, zero-copy processing, or deterministic
Go heap zeroization. Certificate and agent filesystem code still requires review;
this specification does not certify its race safety.

## 9. First Run, Authentication, and Authorization

First-run setup requires a short-lived operator capability and creates the
administrator and permanent installation marker together in PostgreSQL. Setup
completion must survive restarts and prevent competing or repeated installation.
The setup capability is generated into private operator configuration and retrieved
through the local administrator command. It must not appear in logs, URLs, or
public API responses. Deployment secrets remain operator
configuration, not browser-written environment files.

The session design uses short-lived access JWTs tied to database sessions and
opaque refresh tokens stored only as hashes. Refresh rotation is atomic, session
lifetime is bounded, and logout revokes the session, including outstanding access
tokens. Existing legacy refresh tokens are not imported into this session model.

The API sets and clears HttpOnly cookies with SameSite restrictions. Production
uses Secure cookies; local HTTP development is an explicit exception. Browser
mutations require an allowed Origin. Authentication and authorization are server
boundaries; frontend route guards provide user experience, not access control.

Current permissions and account activity must govern access. Invalid credentials,
expired or revoked sessions, and unavailable dependencies must not become success
responses. In-process rate budgets are scoped to client IP and authentication,
read, or mutation traffic. Forwarded client addresses are accepted only from
explicitly trusted proxy peers. Limiter state is bounded and idle entries expire;
health probes remain independent of user traffic. This is not distributed or
per-account abuse protection.

Authenticator secrets are encrypted with account-bound associated data. Pending
MFA sessions cannot access APIs or renew credentials. Password and magic-link
sign-in require the same second-factor completion; email password reset does not
remove it. TOTP time steps and recovery codes are single-use. Local emergency
recovery is restricted to the root operator and revokes old credentials.

The supported installer targets Ubuntu 24.04, stages checksummed versioned
artifacts, and retains the previous release. Existing-schema migration requires a
verified backup. Unknown/newer or modified migration history is rejected. Restore
must pair executable/images, database state, and configuration. Broader tenant
isolation and privileged-agent hardening remain separate release gates.

## 10. Repository and Verification Authorities

- `frontend/`: React/Vite UI and Playwright browser tests.
- `api/cmd/`: API, migration, health, and repository tooling entry points.
- `api/internal/`: HTTP handling, application behavior, database access, and adapters.
- `agent/src/`: Native Rust service, gRPC implementation, and host operations.
- `proto/kari/agent/v1/agent.proto`: Shared RPC schema.
- `deploy/systemd/`: Native-agent service and environment template.
- `toolchain.env`: Exact toolchain and immutable container-image authority.
- `Makefile` and `scripts/`: Shared local and CI verification operations.

`make verify` is the complete verification entry point. For localized changes,
follow AGENTS.md and start with the narrowest relevant check. Privileged agent
host mutation is not dynamically exercised by the container smoke environment.
Use [architecture](ARCHITECTURE.md), [operations](OPERATIONS.md), and the root
[README](../README.md) for the supported runtime boundary and commands.
