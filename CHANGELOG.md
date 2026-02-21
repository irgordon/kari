# Changelog

All notable changes to the Karı Orchestration Engine will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
