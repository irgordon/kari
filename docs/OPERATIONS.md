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
Rate budgets are separate for authentication, reads, and other mutations. Direct
API requests use their TCP peer address. Only peers in `KARI_TRUSTED_PROXY_CIDRS`
may supply `X-Real-IP`; do not trust client-supplied forwarding chains.
Compose reserves `KARI_PROXY_IP` (default `172.30.231.3`) for frontend Nginx and
trusts only that address. Nginx overwrites `X-Real-IP` with its connection peer.
If the default `KARI_PROXY_SUBNET` (`172.30.231.0/24`) overlaps an existing network,
choose a free subnet and a proxy IP within it before startup. Concurrent Compose
installations on one Docker host need distinct subnets. Additional proxy hops
require an explicit trust configuration; they are not trusted automatically.
For an existing installation, use `make compose-down` before `make compose-up`
to recreate the network with these settings. Preserve the PostgreSQL volume;
this network change does not require resetting the database.
Only `KARI_ENV=development` permits non-Secure cookies for local HTTP development.
Restarting the API does not sign users out. Refresh tokens are stored only as hashes,
rotate atomically, and have an absolute seven-day session lifetime. Logout revokes
the current session immediately; other signed-in devices retain their own sessions.

`make topology-smoke` builds a disposable stack and runs the same browser journey
as CI, deleting its database afterward. It requires the pinned Node/npm tools and
downloads Chromium using the lockfile-pinned Playwright release. This command is
also part of `make verify`.

## Account email access and recovery

Configure `KARI_PUBLIC_URL` as the panel's externally reachable HTTPS origin,
without a path, query, or credentials. Set `KARI_SMTP_HOST`, `KARI_SMTP_PORT`
(default `587`), `KARI_SMTP_FROM`, and, when required, `KARI_SMTP_USERNAME` and
`KARI_SMTP_PASSWORD` in the protected environment file. Restart the API after
changing these values. Keep SMTP credentials out of source control.

SMTP requires certificate-verified STARTTLS before authentication or delivery.
Only development mode permits unauthenticated plaintext delivery to a literal
loopback IP for a local mail sink. Each delivery attempt has a ten-second deadline.
This is transactional account email, not hosted customer mailboxes.

Sign in with your password, open **Account**, and request verification. Open the
emailed link in the same browser and confirm while signed in to that account.
Only verified, active accounts can receive password-reset or magic sign-in links.
The login page offers those methods when SMTP is configured. A reset changes the
password and revokes all previous sessions and outstanding email capabilities;
it does not automatically sign the user in. Other accounts are unaffected.

Links expire after 15 minutes, are single-use, and are stored only as hashes.
Tokens travel in URL fragments and are cleared from browser history when opened.
Loading a link does not redeem it: the user must explicitly confirm the action.
Requesting a replacement invalidates the previous link for that purpose. Requests
are limited to one per minute and five per hour for each account and purpose.

Public request responses deliberately do not reveal whether an account exists,
is verified, or whether SMTP delivery succeeded. A bounded queue holds up to 64
requests for one application-owned delivery worker. Delivery failures log only
the purpose, never recipients, tokens, or SMTP responses. There is one delivery
attempt per accepted request, with no automatic retry. Pending requests are not
durable across API restarts; users must request another link if needed. Existing
unexpired links remain redeemable if new email delivery is disabled.

Confirm actual inbox delivery with your SMTP provider before relying on email
recovery. Administrator MFA, offline emergency recovery, and installation/upgrade
safety remain separate P1 work; email links do not provide MFA.
