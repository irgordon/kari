# Kari baseline operations

## Environment setup

Create the Compose environment file:

```bash
cp .env.example .env
```

Development values are intentionally marked as non-production. Production must
provide unique `DB_PASSWORD`, `JWT_SECRET`, and `ENCRYPTION_KEY` values, protect
the environment file, set `KARI_ENV=production`, set `KARI_REQUIRE_AGENT=true`,
and set `KARI_AGENT_SOCKET_DIR=/run/kari`.

## Container topology

Build and start PostgreSQL, migrations, the API, and the frontend:

```bash
make compose-up
```

Check the implemented contracts:

```bash
curl --fail http://localhost:8080/health
curl --fail http://localhost:8080/ready
curl --fail http://localhost:3000/health
```

Stop the topology without deleting the database volume:

```bash
make compose-down
```

## Database initialization

Compose runs `/app/kari-migrate` after PostgreSQL becomes healthy and before the
API starts. For an explicitly supplied database URL, apply the same embedded
chain with:

```bash
DATABASE_URL='postgres://kari:password@127.0.0.1:5432/kari?sslmode=disable' make migrate
```

Applied names and checksums are stored in `schema_migrations`. A checksum change,
missing manifest entry, extra migration, ordering gap, or SQL error fails the
operation. Published v1.0.0 SQL is archived under
`api/internal/db/legacy/v1.0.0/` and is not supported as an upgrade source.

## Native Linux agent

Build the pinned agent on the target Linux host:

```bash
cargo build --locked --release --manifest-path agent/Cargo.toml
```

Install `agent/target/release/kari-agent` at
`/usr/local/libexec/kari/kari-agent`, install
`deploy/systemd/kari-agent.service` at `/etc/systemd/system/kari-agent.service`,
and create `/etc/kari/agent.env` from `deploy/systemd/agent.env.example` with
root-only permissions. Then reload systemd and enable the unit according to the
host's change-management policy.

The baseline verifies the binary and unit statically. It does not dynamically
exercise privileged host mutation in CI.

## First-run administrator and sessions

Set the persistent encryption key and JWT secret before production startup. The
setup form creates only the administrator; it does not rewrite deployment secrets,
database URLs, or environment files. Development permits a fixed all-zero encryption
key when `ENCRYPTION_KEY` is empty; never use this fallback in production.

After `make compose-up`, open the frontend and retrieve the setup token with
`./scripts/compose.sh logs api`. Treat API logs as operator-only: the bootstrap
capability expires in 15 minutes and becomes unusable after setup commits. Restart
an incomplete API with `./scripts/compose.sh restart api` for a new token. Setup
completion persists in PostgreSQL across API/container restarts, independently of
the former `setup.lock` file. Existing databases with users are treated as configured.
The forward migration preserves existing users and password hashes. Legacy refresh
tokens are not imported; existing users must sign in again once after upgrading.

Production must terminate HTTPS at a trusted reverse proxy, preserve the original
Host header, and configure `CORS_ALLOWED_ORIGINS` for the HTTPS frontend origin.
Browser mutations require an allowed Origin; wildcard origins are unsupported.
Only `KARI_ENV=development` permits non-Secure cookies for local HTTP development.
Restarting the API does not sign users out. Refresh tokens are stored only as hashes,
rotate atomically, and have an absolute seven-day session lifetime. Logout revokes
the current session immediately; other signed-in devices retain their own sessions.

`make topology-smoke` builds a disposable stack and runs the same browser journey
as CI, deleting its database afterward. It requires the pinned Node/npm tools and
downloads Chromium using the lockfile-pinned Playwright release. This command is
also part of `make verify`.
