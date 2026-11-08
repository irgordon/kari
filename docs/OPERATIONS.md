# Kari baseline operations

## Environment setup

Generate a new private local development environment (existing files are never overwritten):

```bash
go run ./api/cmd/kari-admin init --development --public-url http://localhost:3000 --output .env
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

After `make compose-up`, open the frontend and retrieve `KARI_SETUP_TOKEN` from
the private environment file. Production operators use `kari-admin setup-token`.
The key is never printed in API logs. It expires 15 minutes after API startup and
becomes unusable after setup commits. Restart an incomplete API with
`./scripts/compose.sh restart api` to restart its validity period. Setup
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
recovery. Email links are a first factor, not MFA.

## Supported installation

Use Ubuntu 24.04 on a dedicated server with rootful Docker Engine 24+, Compose
2.20+, Nginx, systemd, and free UID/GID 1001. Install those OS prerequisites first.
The installer refuses another distribution, an existing installation, occupied
service identities, or pre-existing service/configuration files.

Build a release with `./scripts/build-release.sh VERSION`. It uses digest-pinned
builders and the existing locks, packages actual Linux binaries and images, and
records their SHA-256 checksums, schema head, architecture, and source Git tree.
Source changes during a build abort publication of the local manifest. No Git tag
or remote release is created. A manifest checksum must come from a trusted source;
checksums bundled with an untrusted download do not establish provenance.

Transfer the directory intact. Verify `manifest.json` before running its binary.
Provide a hostname-matching, currently valid certificate and private key; this
installer does not obtain certificates from a public CA automatically.

```bash
sudo ./kari-admin install --bundle /root/kari-release --manifest-sha256 TRUSTED_SHA256 --public-url https://panel.example.com --tls-cert /root/panel.crt --tls-key /root/panel.key
sudo /opt/kari/current/kari-admin setup-token
```

The default installation is `/opt/kari`, mode 0700. The environment file is mode
0600; generated database, JWT, encryption, and setup secrets are independent.
The agent runs natively as root with the API group for socket traversal. API and
frontend HTTP ports bind to loopback. Host Nginx terminates HTTPS and overwrites
the client-IP header; frontend Nginx trusts only the installation's host gateway.
`kari-panel.service` restarts the containers after reboot. Use the generated
environment and installer-managed gateway together; manually changing subnets
requires reviewing both proxy trust configurations.

Open the HTTPS panel, create the administrator, then use **Account**. The checklist
reports database, native-agent, HTTPS, email, and MFA state separately. Development
can omit the agent, but its disconnected state remains visible. SMTP credentials
are configured in `/opt/kari/.env.production`; use `kari-admin restart` afterward.

## MFA and emergency access

Enroll an authenticator from **Account** with your current password. The pending
key expires in ten minutes. Confirm a current code and download the ten recovery
codes immediately: only their hashes remain in the database. Enrollment revokes
old sessions. Login challenges expire in five minutes, permit five attempts, and
cannot authorize API access or refresh a session. New challenges have an account
cooldown; TOTP time steps and recovery codes cannot be replayed. Password and magic
sign-in both complete the second factor before receiving a full session. Password
reset retains MFA. Production workload management is blocked without verified MFA.

If email and the authenticator are unavailable, the local root operator can recover
an existing administrator. Place the replacement password in a root-owned mode-0600
file using a secure editor; do not put it in shell arguments, history, or logs.

```bash
sudo /opt/kari/current/kari-admin recover --email owner@example.com --confirm owner@example.com --password-file /root/new-password --reset-mfa
```

Omit `--reset-mfa` to preserve the authenticator. This command stops the API,
changes credentials transactionally, revokes sessions and email links, and restarts
the API. It is not an HTTP/RPC endpoint. Securely remove the password file after
storing the replacement credentials. Enroll a new factor before resuming production
management. Recovery cannot decrypt data if the encryption key has been lost.

## Backups, upgrades, and restoration

```bash
sudo /opt/kari/current/kari-admin backup
sudo /opt/kari/current/kari-admin upgrade --bundle /root/kari-next --manifest-sha256 TRUSTED_NEXT_SHA256
```

Backups quiesce the API and capture the control-plane database, private environment,
agent configuration, TLS material, and proxy configuration. The backup records the
matching release and checksums. Backups are root-only and contain secrets. Copy
them and the matching release off-host through a protected channel. Customer site
files/databases, scheduling, retention, and encrypted off-host policy are separate
P2/P4 capabilities.

Upgrade preflight rejects incompatible migration history and unsupported
downgrades. A real database backup is mandatory before advancing an existing
schema. Migration is transactional. The previous release and backup remain in
place. A failed upgrade returns nonzero and keeps management stopped when the
state cannot safely resume; do not manually start an older API against a newer
database. The API itself rejects a schema that does not match its release.

Restore deliberately replaces the control-plane database and configuration:

```bash
sudo /opt/kari/current/kari-admin restore --backup /opt/kari/backups/BACKUP_DIRECTORY --confirm PREVIOUS_VERSION
```

The backup must be inside the protected backups directory, its files must pass
checksums, and the retained release must match its schema. Restoration uses the
previous executable/images and database snapshot together. Restoration
does not revive sessions or consumed capabilities: restore revokes all sessions,
email links, and saved recovery codes, rotates the JWT signing secret, and retains
the enrolled authenticator. Sign in with the authenticator and re-enroll to obtain
fresh offline codes, or use local root recovery if the factor was lost. Review
and rotate account passwords as appropriate for the recovery event. A restore failure
leaves management stopped and returns nonzero; repair the cause and retry the same
verified backup. This is same-host upgrade recovery. Replacement-host disaster
recovery still requires provisioning with the original credentials and matching
release; it is not certified by the same-host rehearsal.

For failures, inspect `systemctl status kari-agent kari-panel`, Nginx validation,
and Compose service health. A partial first installation retains its private
configuration for inspection; do not rerun installation over it or delete volumes
to hide a failure. No installer changes an existing owner's installation silently.
