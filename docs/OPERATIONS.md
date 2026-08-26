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
