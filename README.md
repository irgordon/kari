# Kari

Kari is a Go API, React frontend, PostgreSQL database, and privileged Rust host
agent for Linux application orchestration. This recovery baseline defines one
build contract and one supported deployment boundary; it does not claim that
authentication, deployment state, or the privileged RPC surface is production-ready.

## Pinned prerequisites

| Tool | Version |
| --- | --- |
| Go | 1.27.0 |
| Rust, Cargo, rustfmt, Clippy | 1.98.0 |
| Node.js | 24.19.0 |
| npm | 11.17.0 |
| protoc | 21.12 (`libprotoc 3.21.12`) |
| protoc-gen-go | v1.36.6 |
| protoc-gen-go-grpc | v1.5.1 |
| Docker Engine | 24.0.0 or newer |
| Docker Compose | 2.20.0 or newer |

GNU Make, Bash, curl, unzip, Git, and jq are also required. The machine running
browser verification also needs OpenSSL for its disposable setup key. The machine running
verification must have a working Docker daemon. `toolchain.env` is the version
authority consumed by scripts, CI, and container builds.

## Verify a clean checkout

```bash
git clone https://github.com/irgordon/kari.git
cd kari
make verify
```

`make verify` is the only complete verification command. It checks pinned tools,
Go formatting/vet/tests/race detection, Rust format/build/Clippy/tests, frontend
lockfile install/lint/type-check/build, protobuf drift, blank-database migrations,
real repository contracts, Compose/Docker references, topology health, shell
syntax, workflows, and documentation.

## Source identity

Commit authorization uses the portable Git-correlated identity described in
[docs/SOURCE_IDENTITY.md](docs/SOURCE_IDENTITY.md). Generate both the proposed Git
tree object and canonical SHA-256 manifest with:

```bash
./scripts/source-identity.sh --manifest .artifacts/source-identity-v2.manifest
```

Historical filesystem hashes, including the local-v1 `8aaffb0a…` value, are not
authorization identities.

The topology stage runs pinned Playwright browser tests against the real frontend,
API, and a fresh PostgreSQL database. Chromium and its system dependencies are
installed by that stage; no API or database responses are mocked.

## Supported topology

The only supported deployment boundary is:

- PostgreSQL in Docker Compose.
- The Go API and migration binary in Docker Compose.
- The built React frontend served by Nginx in Docker Compose.
- The privileged Rust agent installed natively on a Linux systemd host.

The agent is intentionally absent from `docker-compose.yml`. It mutates host
users, systemd, firewall, proxy, package, and filesystem state and is not treated
as an ordinary container.

## Development startup

```bash
go run ./api/cmd/kari-admin init --development --public-url http://localhost:3000 --output .env
make compose-up
curl --fail http://localhost:8080/health
curl --fail http://localhost:8080/ready
curl --fail http://localhost:3000/health
```

The migration service initializes a blank database before the API starts. The
development environment sets `KARI_REQUIRE_AGENT=false`; readiness therefore
proves the database/API boundary but does not claim a running privileged agent.

Open `http://localhost:3000`. On first run, Kari opens administrator setup.
Read the private setup key from your protected environment file, enter it with
your email and a strong password, then sign in. Production operators retrieve it
with `sudo /opt/kari/current/kari-admin setup-token`. Keys expire 15 minutes after
API startup; the operator can restart the API if setup has not completed. Setup commits the administrator
and a permanent completion marker together in PostgreSQL and needs no restart.

Session cookies are HttpOnly. Refresh tokens rotate on use and expire after seven
days; signing out revokes the session, including its outstanding access tokens.
Production requires HTTPS at the frontend reverse proxy and secure session cookies.

Stop the topology with:

```bash
make compose-down
```

Liveness is `GET /health`. Dependency readiness is `GET /ready`. Production must
set `KARI_REQUIRE_AGENT=true`, causing readiness to require a successful native
agent probe in addition to PostgreSQL.

## Website hosting

On a native-agent installation, open **Websites** to create a static site or
Node.js application from an unauthenticated HTTPS Git repository.
Point the domain to the server, then configure TLS through ACME or a supplied
certificate/key pair. Publication results survive page reloads; the dashboard
reports saved hosting state rather than invented server metrics.

**Manage website** provides Node environment and lifecycle controls, a separate
customer PostgreSQL database, site data files, confined key-only SFTP, and manual
workload backup/restore. **System** reports actual host measurements and service
state and provides validated Nginx reload. Node uses the pinned 24.19.0 runtime;
other application stacks and arbitrary host commands are not supported.

See [website operations](docs/OPERATIONS.md#static-websites) and the
[P2 completion summary](docs/P2_COMPLETION_SUMMARY.md) for supported inputs,
limits, file-by-file reasoning, validation, and remaining production release gates.

## Production boundary

The supported host is a dedicated Ubuntu 24.04 server with rootful Docker,
Compose, Nginx, Git, OpenSSL, OpenSSH server/client tools, systemd, and UID/GID 1001
available. Provide a valid TLS certificate
and root-owned mode-0600 private key for the panel hostname. The installer creates
private credentials, installs the native agent and panel services, binds internal
HTTP ports to loopback, and configures the HTTPS proxy.

Build an immutable, versioned local release from the pinned toolchains:

```bash
./scripts/build-release.sh 1.2.0-rc.1
```

This creates `.artifacts/releases/1.2.0-rc.1/` and prints its manifest SHA-256.
The version is an example, not a published release. Transfer the bundle through
a trusted channel and verify its checksum before executing its administrator binary.

```bash
sudo ./kari-admin install --bundle /root/kari-release --manifest-sha256 TRUSTED_SHA256 --public-url https://panel.example.com --tls-cert /root/panel.crt --tls-key /root/panel.key
sudo /opt/kari/current/kari-admin setup-token
```

Create the administrator in the browser, then open **Account** to complete email
verification and authenticator enrollment. Production management requires a
verified MFA session. Save the one-time recovery codes offline. Password reset
and magic-link login retain the authenticator requirement.

Upgrades reject downgrades, validate migration history, and take a control-plane
backup before migration. If the new release fails, restore matching application,
database, keys, and configuration using the procedure in the operations guide.

The native unit is [deploy/systemd/kari-agent.service](deploy/systemd/kari-agent.service),
with its environment template at
[deploy/systemd/agent.env.example](deploy/systemd/agent.env.example). Detailed
installation and database operations are in [docs/OPERATIONS.md](docs/OPERATIONS.md).

## Known deferred limitations

This milestone does not repair or certify:

1. Broader authorization, tenant-isolation, and privileged-operation hardening.
2. Deployment state transitions, recovery, or terminal outcomes.
3. The breadth or hardening of the privileged-agent RPC surface.

The Compose smoke test compiles the agent and validates its systemd artifacts
statically. It does not execute privileged Linux host mutations.

## Documentation

- [Technical specification](docs/TECHNICAL_SPEC.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Operations](docs/OPERATIONS.md)
- [Agent API](docs/AGENT_API.md)
- [Provider interfaces](docs/PROVIDERS.md)
- [Development](docs/DEVELOPMENT.md)
- [Changelog](CHANGELOG.md)
- [Contributing](CONTRIBUTING.md)

Historical audits and superseded drafts are retained under `docs/archive/` and
are not operational instructions.
