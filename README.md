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

The React source currently has no real frontend test files, so the baseline does
not invent or run a placeholder frontend test command.

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
cp .env.example .env
make compose-up
curl --fail http://localhost:8080/health
curl --fail http://localhost:8080/ready
curl --fail http://localhost:3000/health
```

The migration service initializes a blank database before the API starts. The
development environment sets `KARI_REQUIRE_AGENT=false`; readiness therefore
proves the database/API boundary but does not claim a running privileged agent.

Stop the topology with:

```bash
make compose-down
```

Liveness is `GET /health`. Dependency readiness is `GET /ready`. Production must
set `KARI_REQUIRE_AGENT=true`, causing readiness to require a successful native
agent probe in addition to PostgreSQL.

## Production boundary

Production installation is limited to the native-agent unit and the same Compose
topology. Copy `.env.example` to a protected deployment environment file, replace
all development values, set `KARI_ENV=production`, set `KARI_REQUIRE_AGENT=true`,
and set `KARI_AGENT_SOCKET_DIR=/run/kari`.

The native unit is [deploy/systemd/kari-agent.service](deploy/systemd/kari-agent.service),
with its environment template at
[deploy/systemd/agent.env.example](deploy/systemd/agent.env.example). Detailed
installation and database operations are in [docs/OPERATIONS.md](docs/OPERATIONS.md).

## Known deferred limitations

This milestone does not repair or certify:

1. Authentication and authorization lifecycle behavior.
2. Deployment state transitions, recovery, or terminal outcomes.
3. The breadth or hardening of the privileged-agent RPC surface.

The Compose smoke test compiles the agent and validates its systemd artifacts
statically. It does not execute privileged Linux host mutations.

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Operations](docs/OPERATIONS.md)
- [Agent API](docs/AGENT_API.md)
- [Provider interfaces](docs/PROVIDERS.md)
- [Development](DEVELOPMENT.md)
- [Contributing](CONTRIBUTING.md)

Historical audits and superseded drafts are retained under `docs/archive/` and
are not operational instructions.
