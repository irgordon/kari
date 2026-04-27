# TECHNICAL_SPEC.md: Karı Server Control Panel (Hardened Edition)

## 1. Overview

**Karı** is a modern, high-performance, and secure-by-design server control panel engineered for the workflows of 2026 and beyond. It replaces legacy monolithic control panels with a decoupled, GitOps-driven architecture. Karı manages DNS, web services (Nginx), modern application runtimes (Node.js, Python, Ruby, PHP), automated SSL, and databases through a clean REST API and a lightweight, memory-safe system agent.

## 2. System Architecture

Karı relies on a strict privilege-separation model, splitting operations between an unprivileged API daemon and a root-level system agent.

* **The UI (Decoupled SPA):** Built with React. Communicates exclusively via the public REST API. Uses client-side hooks to manage HttpOnly cookies.
* **The Brain (Go API Daemon):** Runs as an unprivileged user (`kari-api`, usually UID 1001). Handles HTTP REST routing, state management, dynamic RBAC, automated SSL generation, rate limiting, background cron workers, and workflow orchestration.
* **The Muscle (Rust System Agent):** Runs as `root`. Communicates via restricted gRPC over a local Unix socket.
* **The Lockout Handoff:** The socket file is created by root but instantly `chown`'d to the Go API user.
* **Kernel-Level Identity:** The socket enforces `0o660` permissions and utilizes `SO_PEERCRED` to mathematically verify the Go API's UID at the Linux kernel level *before* the gRPC stream is yielded.



---

## 3. Core Design Principles

* **SOLID & Single Layer Abstraction (SLA):** Both the Go and Rust codebases utilize interfaces/traits to strictly decouple business logic from underlying OS commands. The Go API passes *Intents*, and the Rust Agent abstracts the execution.
* **Zero-Trust Input & Execution:** * **No Shells:** The Rust agent strictly uses `std::process::Command` to pass discrete arguments, bypassing `bash` interpolation entirely.
* **Argument Injection Blocks:** Inputs are mathematically validated (e.g., rejecting strings starting with `-` for Git, enforcing regex for domains, bounding TCP ports via `u16` types).


* **Platform Agnosticism:** No hardcoded OS assumptions (e.g., `www-data` or `/var/www/html`). All system paths, users, and groups are injected via environment configuration to ensure compatibility across Alpine, Debian, RHEL, and FreeBSD.
* **Memory & Thread Safety (Backpressure):** Streaming operations (like GitOps builds) utilize bounded `mpsc::channel(512)` channels with non-blocking `.try_send()` relief valves. This mathematically guarantees a slow UI or dropped connection can never cause an Out-Of-Memory (OOM) panic in the root daemon.

---

## 4. PostgreSQL Database Schema

Karı uses PostgreSQL to maintain a strict, relational state of the server, utilizing `JSONB` for flexible configuration and `UUIDv4` for all primary keys to prevent IDOR enumeration attacks.

### Core Tables

* **`users` & `roles` & `permissions`:** Implements granular Dynamic Role-Based Access Control.
* **`domains`:** Maps virtual hosts and SSL state.
* **`applications`:** The core state for modern runtime workflows.
* **`deployments`:** Audit log and state tracking for GitOps workflows.
* **`system_alerts` (The Action Center):** Centralized table for tracking system-wide failures.
* **SLA Hardened:** Utilizes GIN indexes on a `JSONB` metadata column (`@> jsonb_build_object('trace_id', ...)`) for sub-millisecond lookups.
* **Memory Bounded:** Go-side queries enforce a strict mathematical pagination limit (max 100 structs) to prevent GC starvation and DoS vectors.



---

## 5. Application Runtimes & Systemd Jails

Karı leverages Linux's native `systemd` alongside strictly unprivileged OS users to create near-container-level isolation with minimal RAM overhead.

1. **Unprivileged Users:** Every application is assigned a unique, shell-less Linux user (`--shell /bin/false`).
2. **Filesystem Lockdown:** The Rust agent enforces `0o750` permissions natively via kernel syscalls (eliminating slow `chmod` subprocesses).
3. **Systemd Security Directives:** Generated `.service` files are hardened with:
* `ProtectSystem=full` & `ProtectHome=true`
* `PrivateTmp=true` & `NoNewPrivileges=true`
* `PrivateDevices=true` & `RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX` (Network & Device namespaces locked).



---

## 6. GitOps Deployment Architecture

Karı embraces modern CI/CD by pulling code directly from Git providers and utilizing zero-downtime atomic swaps.

1. **The Clone Phase:** Rust shallow-clones the Git repository. Local git hooks are globally disabled (`core.hooksPath=/dev/null`) to prevent RCE from attacker-controlled repositories.
2. **The Build Phase (Privilege Dropping):** Rust uses the Linux `runuser` command to drop privileges from `root` to the unprivileged app user before executing the user's build commands.
3. **Real-Time Observability:** Logs are streamed via gRPC using a backpressure-aware architecture. If the stream lags, intermediate logs are dropped to protect RAM, and a synthetic notification is sent to the UI.
4. **The Zero-Downtime Swap:** Rust atomically updates the `current` symlink.
5. **Hygiene (Symlink-Aware Pruning):** Rust prunes old release directories. The Pruning Engine resolves the `current` symlink target to guarantee the active release is *never* deleted (even after rollbacks), and mathematically restricts deletion to valid 14-digit timestamp directories.

---

## 7. Network & Routing Layer

Karı automatically configures and reloads Nginx to act as a highly performant edge reverse proxy.

* **Template Generation:** Go compiles Nginx configurations in-memory. Domain names are regex-validated to prevent Nginx directive injection.
* **Atomic Validation:** Rust writes the config to a `.tmp` file, runs `nginx -t`, and atomically renames it if valid.
* **Identity Forwarding:** Configurations natively inject `X-Forwarded-For` and WebSocket upgrade headers.
* **Universal Reloading:** `systemctl reload nginx` is used for platform-agnostic process signaling, replacing brittle PID file paths.

---

## 8. Automated SSL Provisioning (Let's Encrypt)

Karı fully automates SSL generation and renewal using a **Volatile-Only Private Key Lifecycle**.

1. **In-Memory Generation:** The Go API generates the ECDSA P-256 private key entirely in CPU registers/RAM.
2. **Platform-Agnostic Challenge:** Go proxies the HTTP-01 challenge writing intent to Rust, maintaining zero-disk-access for the Brain.
3. **Secure Transmission:** Raw PEM bytes are sent over the Unix socket.
4. **Zero-Copy & Zero-Race Storage:** * Rust ingests the secret without cloning the byte array, wrapping it in `secrecy::Secret`.
* Rust uses `OpenOptionsExt` to create the file with `0o600` permissions *at inception*, eliminating the TOCTOU window where keys are world-readable.
* Upon closure drop, Rust automatically zeroizes the memory address (`0x00`).


5. **Best-Effort Go Sweeping:** The Go Brain manually overwrites the plaintext slices with `0` to shrink the Garbage Collector attack window.

---

## 9. Security, Authentication, and Gateway Defenses

* **Token Management:** Uses short-lived access JWTs and long-lived Refresh tokens stored exclusively in `HttpOnly`, `Secure` cookies.
* **Input Validation:** Incoming JSON is validated via `go-playground/validator`.
* **Rate Limiting:** In-memory token-bucket rate limiting applied per IP.
* **Error Scrubbing:** Git clones that fail with embedded Personal Access Tokens (PATs) are scrubbed before errors are returned to the UI or Action Center, preventing credential leakage in logs.

---

## 10. Monorepo File Structure

*(The file structure remains the same as previously defined, with the hardened files residing in `agent/src/sys/` and `api/internal/adapters/`)*
