# Changelog

All notable changes to the Karı Orchestration Engine will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### 🛡️ Security & Cryptography (Zero-Trust)

- **CORS Hardening**: Fixed a critical vulnerability where `AllowedOrigins` used a wildcard (`*`) combined with `AllowCredentials: true`. The configuration now strictly enforces a whitelist of origins loaded from the `CORS_ALLOWED_ORIGINS` environment variable, preventing potential Cross-Origin Resource Sharing attacks.
- **Nginx Configuration Injection Fix**: Hardened `stream_deployment` in the Rust Agent to strictly validate `domain_name` and `app_id` using `validate_identifier` before generating Nginx configuration, preventing arbitrary directive injection (e.g., via `;` or `{`).

### 🧪 Testing & Reliability

- **Agent Security Helpers**: Added comprehensive unit tests for `secure_join` and `validate_identifier` in the Rust Agent, covering path traversal and identifier validation.
- **Improved Zero-Trust Validation**: Hardened `validate_identifier` to proactively reject path traversal sequences (`..`), even when dots are otherwise allowed in the identifier.

### 🧹 Code Health

- **Frontend Type Safety**: Introduced strict TypeScript definitions for `Deployment` entities in the SvelteKit frontend, replacing `any` types to prevent runtime errors and improve developer experience.

## [1.0.0] - 2026-02-22

### 🎉 First Major Release: The Manifesto Realized

Version 1.0.0 marks the completion of the foundational Karı architecture, strictly adhering to the Karı Manifesto. This release solidifies the Zero-Trust execution model, the SvelteKit "Glass-Bento" frontend, and mathematically proven privacy invariants.

### 🎨 UI/UX Refinement (The "Glass-Bento" Aesthetic)

- **Fluid Layout**: Completely redesigned the SvelteKit frontend into a modern, recessed, lightweight dashboard.
- **Translucency & Glows**: Applied `backdrop-blur` capabilities and "Accent Glows" (`shadow-indigo-500/20`) to highlight active system states.
- **Deep Slate Terminal**: Embedded `xterm.js` deployment telemetry in a recessed, shadowy container with customized Webkit scrollbars and responsive fitting.
- **Bento Grid**: Visualized CPU/RAM load using "Smooth Pills" with indigo-to-violet gradients, replacing harsh metric blocks.

### 🛡️ Security & Cryptography (Zero-Trust)

- **BIP-39 Master Key Generation**: Introduced a secure Onboarding Wizard that generates an AES-256-GCM master key and provides a 12-word recovery phrase.
- **Setup Lock Mechanism**: Implemented `setup.lock` physical file checks to permanently seal the initialization API endpoints once the cluster is provisioned.
- **Authorized Associated Data (AAD)**: All database secrets (e.g., environment variables) are now authenticated against their originating `AppID` via AAD, preventing ciphertext swapping.
- **Memory Scraping Mitigation**: Applied `Zeroize` traits to Rust environment variable maps and SSL private keys, purging them from RAM instantly after consumption.
- **Granular Scoping**: JWT authorization now enforces Defense-in-Depth. The Go Router intrinsically rejects all `POST/PUT/DELETE` requests lacking mutation payload scopes, regardless of route handlers.

### ⚡ SLA & Resilience

- **Real-time Telemetry Backpressure**: Deployment Server-Sent Events (SSE) now use strictly-timed `select` channels. Slow downstream consumers are dynamically dropped, guaranteeing the Go Brain never blocks on log streaming.
- **Strict Context Propagation**: The Rust Agent now binds all long-running asynchronous threads (like `git clone`) to `kill_on_drop(true)`. If a context is cancelled by the Go client, sub-processes receive instant OS-level `SIGKILL`s.
- **Database Pooling**: Hardened Postgres connection latency with explicit `pgxpool` limits (`MaxConns=50`, bounds on lifetime/idleness) to prevent FD or port exhaustion.

### 📦 SOLID Design & Architecture

- **Dependency Inversion**: Refactored worker dependencies (e.g., telemetry hubs vs broadcasters) to rely entirely on interfaces (`Broadcaster`), enforcing pure domain abstractions.

## [0.0.3] - 2026-02-21

### 🛡️ Security & Cryptography

- **Memory Hygiene**: Implemented `ProviderCredential` using the `secrecy` crate. All sensitive tokens (SSH keys, SSL private keys) are now zeroized in RAM immediately after use, preventing memory-scraping attacks.
- **Physical Disk Scrubbing**: Updated `GitManager` to physically overwrite temporary SSH key files with zeroes on the SSD before unlinking, preventing forensic recovery of transient credentials.
- **Anti-Enumeration**: Hardened `AuthService` with dummy Bcrypt hashing to equalize response times, neutralizing timing attacks used for user discovery.
- **AEAD Integrity**: Upgraded `AESCryptoService` to use AES-256-GCM with **Associated Data (AAD)**. This cryptographically binds encrypted secrets to specific AppIDs/UserIDs, preventing "Confused Deputy" data swapping.
- **Credential Hashing**: Opaque refresh tokens are now stored as SHA-256 hashes in PostgreSQL; a database leak no longer results in session hijacking.

### 🏗️ Muscle (Rust Agent) Improvements

- **Polymorphic Proxying**: Consolidated `ProxyManager` to support both **Nginx** and **Apache** via a single trait. Added boot-time auto-discovery to detect the host's web server.
- **Process Group Isolation**: Build commands now run in a dedicated Linux Process Group with `kill_on_drop(true)`. If a gRPC stream disconnects, the parent and all child processes (e.g., `npm install`) are terminated instantly.
- **Race Condition (TOCTOU) Fixes**:
  - `SslEngine` now uses `OpenOptionsExt` to set `0o600` permissions at the moment of file creation.
  - `JailManager` and `CleanupManager` now use `fs::canonicalize` to resolve absolute physical paths, defeating relative symlink-based escape attempts.
- **Argument Injection Defense**: Implemented a strict whitelist and metacharacter blacklist (`;|&`) for package management and build commands.

### ⚙️ Brain (Go API) & Orchestration

- **Optimistic Concurrency (OCC)**: Introduced version-based locking in the `SystemProfile` domain and PostgreSQL repository. Prevents "Lost Update" scenarios when multiple admins modify settings simultaneously.
- **Kernel-Level Auth**: Reinforced `SO_PEERCRED` verification. The Agent now strictly validates the numeric UID of the Go API container, rejecting any cross-container communication not originating from the Brain.
- **Strict Typing**: Refactored all internal boundaries to use `std::path::Path` instead of raw strings, pushing path sanitization to the outermost edge of the gRPC server.

### 🚀 Infrastructure

- **Zero-Trust Networking**: Refactored `docker-compose.yml` to use an internal `backplane` network with no internet access for the Database and Agent.
- **Cryptographic Bootstrapping**: Added a secure bash generator (`gen-secrets.sh`) using `/dev/urandom` to produce high-entropy 256-bit encryption keys and JWT secrets.

## [v0.0.2] - 2026-02-21

### 🛡️ Security

- **Path Traversal Protection**: Implemented strict `secure_join` validation in the Rust Agent (`server.rs`) to mathematically prevent directory traversal attacks during deployment and teardown operations.
- **Strict Identity Verification**: The Rust Agent now mandates a deterministic `KARI_API_UID` environment variable on boot. Removed the brittle `1001` fallback to prevent `SO_PEERCRED` bypasses.
- **Argument Injection Defense**: Hardened `execute_package_command` in the Rust Agent to proactively reject shell metacharacters (`;&|`) in package manager arguments.
- **Kernel-Level Sandboxing**: Upgraded the Go API's bare-metal systemd unit (`install.sh`) to use `ProtectSystem=strict`, `RestrictSUIDSGID=true`, and an empty `CapabilityBoundingSet`, eliminating host filesystem access.
- **Immutable Network Boundaries**: Removed mutable `AgentSocketPath` from the PostgreSQL `SystemProfile` domain entity to prevent privilege escalation via database injection.
- **UI Information Leakage**: Configured Vite (`esbuild.drop`) to automatically strip all `console.log` and `debugger` statements from the production SvelteKit build.
- **Cryptographic Enforcement**: The Go API now halts on boot (`log.Fatal`) if `JWT_SECRET` is missing in production, preventing weak default token signing.

### 🏗️ Architecture & SLA

- **Single Layer Abstraction (SLA)**: Completely removed OS-specific filesystem paths (e.g., `/etc/nginx`) from the Go Brain (`config.go`). All host execution details are now strictly delegated to the Rust Muscle (`config.rs`) using type-safe `PathBuf` structures.
- **Multi-Stage Containerization**: Engineered a hardened, Zero-Trust `Dockerfile` for the SvelteKit frontend using `@sveltejs/adapter-node`, reducing image bloat and enforcing a rootless `node` execution environment.
- **Deterministic Teardown**: Refactored the Rust Agent's `delete_deployment` flow to surface `Result` errors natively. If a systemd service fails to stop, the agent now deterministically aborts the deletion rather than swallowing the error and leaving zombie processes.
- **Platform-Agnostic Cross-Compilation**: Replaced native Cargo build steps with `cross` in GitHub Actions to guarantee deterministic, C-dependency-safe builds for `aarch64-unknown-linux-musl` (ARM64).

### 🚀 Added

- **Automated GHCR Delivery**: Upgraded the CI/CD pipeline (`release.yml`) to automatically build and push multi-architecture (AMD64/ARM64) Docker images to the GitHub Container Registry via QEMU and Buildx caching.
- **Developer DX Scripts**:
  - `dev.sh`: Native host execution with dynamic UID injection and mocked OS paths for instant frontend HMR without `sudo`.
  - `up.sh`: Full Docker Compose integration testing with automated cryptographic `.env` bootstrapping.
- **Documentation Suite**: Added comprehensive architectural documentation in `docs/`:
  - `ARCHITECTURE.md`: Details the 3-tier Zero-Trust boundary.
  - `AGENT_API.md`: Formalizes the gRPC Protocol Buffer schema.
  - `PROVIDERS.md`: Outlines plugin interfaces for proxies and SSL.
  - `SYSTEM_CHECK.md`: Distro compatibility and bare-metal dependency matrix.

### 🎨 Frontend & UI

- **Type-Safe Design System**: Centralized brand typography and semantic colors in `tailwind.config.js` to ensure zero-bloat JIT CSS compilation.
- **Performance**: Enabled build-time precompression (`.gz` and `.br`) via `svelte.config.js` to eliminate CPU overhead during static asset serving.
- **Network Agnosticism**: Enforced `0.0.0.0` host binding and fallback polling in `vite.config.ts` to guarantee Hot Module Replacement (HMR) functions flawlessly inside Docker networks across Windows, macOS, and Linux hosts.
