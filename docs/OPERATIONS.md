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

## Static websites

The static hosting path supports prepared static websites from public HTTPS Git
repositories. It requires the native agent and host Nginx; the container-only
development smoke environment does not provide host publication. Use one API
instance and one native agent on the supported Ubuntu server.

1. Sign in, complete required MFA, and open **Websites**.
2. Enter the website hostname, public HTTPS repository URL, and branch. The panel
   hostname is reserved. Credentials in repository URLs are rejected.
3. Put the finished `index.html` and public assets in the repository root, then
   select **Publish latest branch**. No build commands, package installation,
   submodules, private Git credentials, or runtime processes are supported here.
4. Point the hostname's A/AAAA records to this server. Remove conflicting AAAA
   records. Confirm the public HTTP website before requesting TLS.
5. Select **Configure TLS**. Leave both PEM fields empty for ACME, explicitly
   accept the subscriber agreement, and use an account with a verified email.
   Inbound ports 80 and 443 must reach host Nginx. Alternatively supply a valid,
   hostname-matching PEM chain and matching key. Imported certificates must be
   trusted by your visitors; accepting a self-signed test certificate does not
   establish public trust.

All nonhidden repository files are public. Never commit credentials or private
configuration to the published repository. Hidden files are excluded, visible
symlinks and special files are rejected, and publication is limited to 10,000
entries and 100 MiB. Fetching runs as a separate site identity under a bounded
systemd service, with hooks disabled and without executing repository code.
This narrow path does not certify the older general-purpose agent RPC surface.

Publication continues when a browser closes. **View latest publication** restores
its persisted result and bounded recent log history. Failed fetches leave the
previous release active. A panel restart before acknowledgement is reported as
an interrupted failure; inspect the website and explicitly publish again. A
failed acknowledgement is not proof that no host change occurred.

Nginx configuration is tested before reload, and TLS success waits until the
expected certificate is served locally. Failed candidate activation retains the
previous configuration. Certificates and releases remain in the root-managed
site directory under `/var/www/kari/sites/`; monitor disk use. Automatic retention
and certificate renewal are not implemented. Use **Configure TLS** again to renew
before expiry. The dashboard warns when saved expiry is within 30 days, but it is
not a live availability, DNS, resource, or certificate-monitoring service.

The API uses owner-scoped `/api/v1/sites` operations and application/domain
permissions. Deprecated domain/deployment pages redirect to Websites; their old
unwired HTTP paths are not promoted into supported APIs. ACME uses the existing
pinned Lego dependency and optional operator `ACME_DIRECTORY_URL`; the default is
the Let's Encrypt production directory. No public certificate issuance was
performed in the disposable local rehearsal.

Select **Manage website** for the workload controls below. Control-plane backups
and workload snapshots have different scopes; neither replaces off-host disaster
recovery. Production release gates remain in force beyond feature completion.

## Node.js applications

Choose **Node.js 24.19.0** when creating a website. The repository root must contain
`package.json` and `package-lock.json`. Karı runs `npm ci --ignore-scripts`, then
`npm run build --if-present`, inside constrained unprivileged containers. Each
step has a two-minute limit, 512 MiB memory, one CPU, and 128 processes. Dependency
lifecycle scripts are disabled. Repositories needing other toolchains or native
install hooks are outside this supported stack.

The runtime executes `npm start`, listens on `PORT=3000` at `0.0.0.0`, and must
serve a successful HTTP response at `/`. Published source is mounted read-only;
write persistent application data under `/data` (`KARI_DATA_DIR`). Temporary
files belong under `/tmp`. Runtime resource limits are fixed at 512 MiB, one
CPU, and 128 processes. Runtime and PostgreSQL images come from the existing
digest pins; no arbitrary image or host command is accepted from the browser.

Before first publication, open **Manage website** to set custom environment
variables and optionally create the database. Environment input replaces the
entire custom map, is write-only, and is stored in root-private host files.
Use uppercase `KEY=value` entries, at most 64 variables and 8 KiB per value.
`PORT`, `HOST`, `NODE_ENV`, `KARI_DATA_DIR`, and `DATABASE_URL` are managed values.
Publish again to apply environment/database changes. Start, stop, and restart
control the current published container; they do not rebuild it.

The new container must serve `/` before proxy activation. Nginx must acknowledge
the new route before the old container is retired. The reserved
`/.__kari_route` HTTP location is a loopback-only activation probe; remote
requests receive 404. TLS activation additionally checks the served certificate.
Recent application logs include bounded stdout and stderr (up to 32 KiB each),
with explicit truncation. Application code must avoid logging secrets.

## Customer databases

**Create customer database** creates a PostgreSQL instance on this website's
Docker network, separate from the panel and other sites, with no published
database port. The application role `site` is not a superuser and cannot create
roles or databases. The database is `site`, at `database:5432` within the site's
network. Karı injects its generated credential as `DATABASE_URL` when publishing.
Passwords are not returned in backup results or displayed in list responses.
Run customer schema migrations through the application; this panel does not
provide a browser SQL editor or remote PostgreSQL port.

Database deletion requires application-delete permission and typed hostname
confirmation. Back up first: deletion removes that database's files. A failed
creation remains visible for inspection; explicitly delete/recreate it rather
than assuming a partial instance is usable. After a create/delete operation,
republish the Node application to refresh its connection environment.

## Site files and SFTP

The browser file manager operates only inside the site's persistent data
directory: `/data` for Node, public `/files/` URLs for static sites. It supports
directory listing/creation, UTF-8 text editing, and file deletion. Reads/writes
are limited to 256 KiB and directory listings to 200 entries; use SFTP for larger
or binary files. Directory deletion is not exposed by the browser. Published Git
releases are read-only; update the repository and republish to change source.

**Enable or replace SFTP key** accepts one Ed25519 or RSA OpenSSH public key,
without authorization options. Connect on port 22 using the displayed site
username and panel hostname. The session starts in `/files` inside the site's
chroot. Shell, forwarding, and password authentication are disabled. Root owns
the chroot, configuration, and published releases. Public authorization keys
live under `/etc/ssh/kari_keys/`; private application settings and backups stay
unreadable to the SFTP identity. The installer requires OpenSSH server/client
tools; existing operator SSH settings and logins are not replaced.

The API checks ownership and permissions; the agent checks site/domain binding
and scopes paths again. Browser file access uses descriptor-relative no-follow
operations to contain SFTP-created symlinks. Static Nginx rejects symlinks whose
owner differs from the target. Disabling key access removes authorization and
terminates that site's SSH sessions without stopping its application.

## Manual workload backup and restore

**Create workload backup** quiesces the application and existing SFTP sessions,
copies data/release files, creates an actual PostgreSQL dump when a customer
database exists, and saves active/pending environment and certificate metadata.
The application resumes with a health check and the current SFTP authorization
is restored. Backups are root-private, checksummed, and listed only for the owner.
Each release/data tree is limited to 512 MiB and 100,000 entries; database dumps
are limited to 512 MiB. At most 100 snapshots per site are retained. Delete old
snapshots explicitly and monitor available disk space.

**Restore** requires application-delete permission and typed website hostname
confirmation. It verifies the snapshot before replacing current files, customer
database, runtime settings, release, and certificate. Database credentials rotate
to a fresh value and are injected into the restored runtime; old passwords and
revoked SFTP keys are not revived. Current SFTP authorization is retained rather
than rolled back. Saved external application credentials still require operator
review after recovery.

A failure before replacement retains current state. Failure during replacement
leaves the application/SFTP stopped for operator repair and explicit retry; do
not repeatedly click restore or manually start a mismatched release. An uncertain
acknowledgement requires checking host state. Snapshots remain intact for repair.
An expired saved certificate is reported as expired and needs renewal.

These are same-host workload snapshots, not the panel database/key backup made
by `kari-admin backup`. Schedules, retention automation, encrypted off-host copies,
replacement-host restore orchestration, and workload migration remain later work.

## Server operations

**System** reports native CPU, memory, root filesystem space, uptime, and fixed
service states when refreshed. Missing measurements are reported as unavailable.
Application logs and lifecycle controls are per website. Server-management
permission is required. Only configuration-validated Nginx reload is exposed;
Docker/SSH/agent lifecycle, arbitrary commands, packages, and reboot remain local
operator responsibilities. The former unsupported global settings page redirects
to these implemented controls and does not claim to synchronize resource policy.

## Supported installation

Use Ubuntu 24.04 on a dedicated server with rootful Docker Engine 24+, Compose
2.20+, Nginx, Git, OpenSSL, OpenSSH server/client tools, systemd, and free UID/GID
1001. Install those OS prerequisites first.
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
