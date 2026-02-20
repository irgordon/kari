# TECHNICAL_SPEC.md: Karı Server Control Panel

## 1. Overview
**Karı** is a modern, high-performance, and secure-by-design server control panel engineered for the workflows of 2026 and beyond. It replaces legacy monolithic control panels with a decoupled, GitOps-driven architecture. Karı manages DNS, web services (Nginx), modern application runtimes (Node.js, Python, Ruby, PHP), automated SSL, and databases through a clean REST API and a lightweight, memory-safe system agent.

## 2. System Architecture
Karı relies on a strict privilege-separation model, splitting operations between an unprivileged API daemon and a root-level system agent.



* **The UI (Decoupled SPA):** Built with SvelteKit. Communicates exclusively via the public REST API. Uses server-side hooks to manage HttpOnly cookies.
* **The Brain (Go API Daemon):** Runs as an unprivileged user (`kari-api`). Handles HTTP REST routing, state management, dynamic RBAC, automated SSL generation, rate limiting, background cron workers, and workflow orchestration.
* **The Muscle (Rust System Agent):** Runs as `root`. Communicates via restricted gRPC over a local Unix socket (`0o660` permissions). Executes OS-level mutations safely without ever invoking a shell (bypassing bash injection entirely).

---

## 3. Core Design Principles
* **SOLID & Single Layer Abstraction (SLA):** Both the Go and Rust codebases utilize interfaces/traits to strictly decouple business logic from underlying OS commands or database SQL queries.
* **Secure by Design:** Zero shell execution. The Rust agent uses `std::process::Command` to pass explicit arguments. In-memory SSL key generation prevents arbitrary file read exploits.
* **Privacy-First:** Audit logs and tenant data are strictly isolated via JWT subject claims. Tenants can only query resources they physically own; admins have dedicated endpoints for system-wide alerts.
* **Zero-Downtime Resilience:** Configurations and application deployments utilize atomic swaps (`ln -sfn` and `std::fs::rename`) to ensure live websites never drop traffic during updates.

---

## 4. PostgreSQL Database Schema
Karı uses PostgreSQL to maintain a strict, relational state of the server, utilizing `JSONB` for flexible configuration and `UUIDv4` for all primary keys to prevent IDOR enumeration attacks.



### Core Tables
* **`users`:** Manages tenant accounts and links to the dynamic RBAC roles.
* **`roles` & `permissions` & `role_permissions`:** Implements Dynamic Role-Based Access Control. Replaces rigid ENUMs with granular rights (e.g., `applications:write`, `server:restart`). Protects the Super Admin from accidental lockout.
* **`domains`:** Maps virtual hosts and SSL state (`none`, `active`, `renewing`, `failed`).
* **`applications`:** The core state for modern runtime workflows (repo URL, build commands, internal ports, JSONB env vars).
* **`deployments`:** Audit log and state tracking for GitOps workflows, linked by `trace_id` for real-time WebSocket streaming.
* **`audit_logs`:** Centralized table for tracking tenant mutations and system-wide alerts (e.g., cron job failures) with `JSONB` detail payloads and severity levels.

---

## 5. Application Runtimes & Systemd Jails
Legacy panels use CGI wrappers or bulky Docker containers. Karı leverages Linux's native `systemd` alongside strictly unprivileged OS users to create near-container-level isolation with minimal RAM overhead.



1. **Unprivileged Users:** Every application is assigned a unique, shell-less Linux user (e.g., `kari-app-1234abcd`).
2. **Filesystem Lockdown:** The Rust agent enforces `750` permissions, ensuring applications cannot read other tenants' files.
3. **Systemd Security Directives:** The generated `.service` files enforce `ProtectSystem=full` (making `/usr` and `/etc` read-only), `PrivateTmp=true` (isolated temp directories), and `NoNewPrivileges=true`.

---

## 6. GitOps Deployment Architecture
Karı embraces modern CI/CD by pulling code directly from Git providers and utilizing zero-downtime atomic swaps.



1. **The Clone Phase:** The Rust agent shallow-clones the Git repository into a timestamped `/releases/YYYYMMDDHHMMSS` directory.
2. **The Build Phase (Privilege Dropping):** Rust uses the Linux `runuser` command to drop privileges from `root` to the unprivileged app user before executing the user's `npm run build` or `pip install` command.
3. **Real-Time Observability:** Rust streams `stdout`/`stderr` line-by-line via gRPC to the Go API, which multiplexes it to the SvelteKit UI via WebSockets for rendering in an XSS-proof `xterm.js` terminal.
4. **The Zero-Downtime Swap:** Upon a successful build, Rust atomically updates the `current` symlink and restarts the `systemd` service. If the build fails, the symlink is untouched, and the live site stays up.
5. **Hygiene:** The Rust agent automatically prunes old release directories (keeping the last 5) and generates `logrotate.d` configurations to compress and clear application logs over 14 days.

---

## 7. Network & Routing Layer
Karı automatically configures and reloads Nginx to act as a highly performant edge reverse proxy.

* **Template Generation:** The Go API compiles Nginx configuration blocks in-memory (using `text/template`), passing them to Rust via gRPC.
* **Atomic Validation:** Rust writes the config to a `.tmp` file, runs `nginx -t`, and only atomically renames it into production if the syntax is valid, preventing total webserver crashes.
* **Identity Forwarding:** Configurations natively inject `X-Forwarded-For`, `X-Real-IP`, and WebSocket upgrade headers.

---

## 8. Automated SSL Provisioning (Let's Encrypt)


Karı fully automates SSL generation and renewal without exposing private keys to the API daemon.

1. **In-Memory Generation:** The Go API negotiates the HTTP-01 ACME challenge via the webroot method and generates the private key entirely in memory.
2. **Secure Transmission:** The raw PEM bytes are sent over the Unix socket to Rust.
3. **Root-Only Permissions:** Rust writes `privkey.pem` with strict `0600` permissions. If the Go API is ever compromised, the attacker cannot read the server's private keys.
4. **Background Cron Worker:** A native Go worker wakes up every 24 hours, parses the `fullchain.pem` expiration dates from the disk, and automatically renews certificates 30 days before expiration, logging any failures to the Action Center via the Audit Service.

---

## 9. Security, Authentication, and Gateway Defenses
Karı treats the Go API as an impenetrable gateway.

### 9.1. Token Management & SvelteKit Integration
* **Browser UI:** Uses short-lived access JWTs and long-lived Refresh tokens stored exclusively in `HttpOnly`, `Secure` cookies. SvelteKit's `hooks.server.ts` intercepts these to manage sessions, immunizing the platform from XSS token theft.
* **CLI/Programmatic:** Uses cryptographically secure Personal Access Tokens (PATs) passed via the `Authorization: Bearer` header.

### 9.2. Gateway Protections
* **Input Validation:** All incoming JSON payloads are aggressively validated using Go struct tags (`go-playground/validator`) before business logic is executed.
* **Rate Limiting:** In-memory token-bucket rate limiting (via `golang.org/x/time/rate`) is applied per IP address to prevent brute-force attacks and API abuse.
* **Friendly UX:** Technical system errors are intercepted by a centralized Error Mapper, returning HTTP-appropriate status codes with polite, non-technical actionable hints for the UI.

---

## 10. Local Development & Distribution

### 10.1. Local DX (`scripts/dev.sh`)
Developers can run the entire monorepo locally via a single Bash script. It utilizes Docker Compose for the Postgres database, overrides the Unix socket path to a local `.local-dev/` directory (allowing the Rust agent to run rootless on Mac/Windows), manages all processes, and multiplexes the logs into a single terminal window.

### 10.2. CI/CD Release Pipeline
Karı is distributed as highly optimized, statically linked binaries via an automated GitHub Actions pipeline.
* **Dependency Scanning:** The pipeline runs `govulncheck`. If any third-party Go package has a known CVE, the release instantly fails.
* **Compilation:** Go is compiled natively (`CGO_ENABLED=0`). Rust is cross-compiled targeting `musl` (`x86_64-unknown-linux-musl`) to guarantee execution on any Linux host without glibc conflicts.
* **Idempotent Installer:** Deployed via `curl -sSL ... | sudo bash`, which handles OS detection, dependency bootstrapping, user creation, resilient artifact fetching, and systemd daemon registration.

---

## 11. Monorepo File Structure
```text
kari/
├── .github/workflows/release.yml       # CI/CD pipeline
├── agent/                              # The Muscle (Rust gRPC Daemon)
│   ├── build.rs                        
│   ├── Cargo.toml                      
│   └── src/
│       ├── main.rs                     # Entrypoint, secure Unix socket binding
│       ├── server.rs                   # gRPC implementation & orchestration
│       └── sys/                        # System Integrations (SOLID SLAs)
│           ├── build.rs                # Privilege dropping (runuser) for GitOps builds
│           ├── cleanup.rs              # Old release pruning
│           ├── git.rs                  # Safe Git cloning
│           ├── jail.rs                 # Linux user creation and filesystem lockdown
│           ├── logs.rs                 # logrotate.d generation
│           └── systemd.rs              # Generates secure systemd unit files
├── api/                                # The Brain (Go REST API)
│   ├── cmd/kari-api/main.go            # App entrypoint
│   ├── internal/
│   │   ├── adapters/                   # Concrete implementations
│   │   │   ├── nginx_manager.go        
│   │   │   └── acme_provider.go        
│   │   ├── api/                        # HTTP Transport Layer
│   │   │   ├── handlers/               # Route controllers (application, audit, websocket)
│   │   │   ├── middleware/             # Auth, RBAC, Rate Limiting, Logging
│   │   │   └── router/                 # Chi mux initialization
│   │   ├── core/                       # Business Logic (SOLID)
│   │   │   ├── domain/                 # Structs & Repository Interfaces
│   │   │   ├── services/               # Orchestrators (SSL, Audit, User/RBAC)
│   │   │   └── utils/                  # Utilities (e.g., Cert Parser)
│   │   ├── db/                         # Database Layer
│   │   │   ├── migrations/             # SQL Schema files (001, 002, 003)
│   │   │   └── postgres/               # Concrete SQL implementations
│   │   ├── workers/                    # Background Cron Jobs
│   │   │   └── ssl_renewer.go          
│   │   └── grpc/                       # Generated Go gRPC client
├── frontend/                           # The UI (SvelteKit SPA)
│   ├── package.json
│   └── src/
│       ├── hooks.server.ts             # Server-side JWT gatekeeper
│       ├── lib/                        
│       │   ├── api/                    # Frontend SLA Layer (fetch wrappers, WS class)
│       │   └── components/             # SRP UI Components (Terminal, Action Center)
│       └── routes/                     # Filesystem Routing
├── proto/                              
│   └── kari/agent/v1/agent.proto       # gRPC strict types
├── scripts/                            
│   ├── dev.sh                          # Local multiplexing bootstrapper
│   └── install.sh                      # Production bash installer
├── docker-compose.yml                  # Local PostgreSQL
└── README.md
