# Changelog
All notable changes to the Karı Orchestration Engine are documented in this file.

This project follows the Keep a Changelog format and adheres to Semantic Versioning.

## [1.1.0] - 2026-04-27
### Frontend Migration (React Rewrite)
- Replaced the entire SvelteKit frontend with a new React + Vite implementation.
- Ported all application pages: Login, Dashboard, Settings, Deployments, Domains, Roles, Audit, System, and Users.
- Removed all SvelteKit dependencies, build artifacts, and configuration files.
- Updated all documentation (README, DEVELOPMENT, ARCHITECTURE, TECHNICAL_SPEC) to reflect the React-based architecture.
- Updated GitHub workflows (CodeQL, Verify) to use Node 22 and React build steps.
- Removed the legacy `frontend_svelte_legacy` directory after full migration.
- Ensured deterministic frontend builds using `npm ci` and Vite’s production bundling.
- Added new API client helpers (`apiPut`, `apiDelete`) and unified API access patterns.
- Updated Makefile to include deterministic frontend setup and build steps.

### Workflow and CI Modernization
- Updated CodeQL workflow to use stable Go, Rust, and Node toolchains.
- Updated Verify workflow to use Node 22 and correct Go/Rust versions.
- Removed all SvelteKit-specific CI logic and caches.
- Ensured `make dev` is the single unified verification entrypoint.

---

## [1.0.0] - 2026-02-22
### First Major Release
- Completion of the foundational Karı architecture, aligned with the Karı Manifesto.
- Zero-Trust execution model established.
- SvelteKit “Glass-Bento” frontend introduced.
- Privacy invariants formalized.

### UI/UX Refinement
- Redesigned SvelteKit frontend with translucent layout, glow accents, and responsive terminal integration.
- Introduced Bento Grid metrics visualization.

### Security and Cryptography
- Added BIP-39 master key generation.
- Implemented setup lock mechanism.
- Enforced AAD binding for encrypted secrets.
- Added Zeroize traits for sensitive memory.
- Strengthened JWT authorization boundaries.

### SLA and Resilience
- Added real-time telemetry backpressure.
- Enforced strict context propagation.
- Hardened database pooling.

### Architecture
- Introduced dependency inversion patterns.

---

## [0.0.3] - 2026-02-21
### Security and Cryptography
- Added memory hygiene via secrecy crate.
- Implemented physical disk scrubbing for temporary SSH keys.
- Hardened AuthService against timing attacks.
- Upgraded AES encryption with AAD.
- Added hashed refresh token storage.

### Rust Agent Improvements
- Added polymorphic proxying.
- Enforced process group isolation.
- Fixed TOCTOU race conditions.
- Added argument injection defenses.

### Go API and Orchestration
- Added optimistic concurrency control.
- Enforced SO_PEERCRED validation.
- Strengthened path typing.

### Infrastructure
- Added Zero-Trust networking.
- Added cryptographic bootstrapping scripts.

### Added
- Automated GHCR delivery.
- Developer DX scripts.
- Documentation suite.
- Type-safe design system.
- Build-time precompression.
- Network-agnostic HMR configuration.

---

## [v0.0.2] - 2026-02-21
### Security
- Implemented strict secure_join validation.
- Enforced deterministic KARI_API_UID.
- Added argument injection defenses.
- Hardened systemd unit.
- Removed mutable AgentSocketPath.
- Stripped console logs from production builds.
- Enforced JWT_SECRET presence in production.

### Architecture and SLA
- Removed OS-specific filesystem paths from Go API.
- Added multi-stage containerization.
- Improved deterministic teardown.
- Added platform-agnostic cross-compilation.

### Added
- Automated GHCR delivery.
- Developer scripts.
- Documentation suite.
- Type-safe design system.
- Performance improvements.
- Network-agnostic HMR.

---

## [0.0.0] - Initial Development Snapshot
### Security and Cryptography
- Fixed credential leak in setup wizard.
- Hardened CORS configuration.
- Added strict Nginx configuration validation.
- Added strict domain validation.

### Performance and Reliability
- Optimized environment bulk import.
- Improved AppMonitor concurrency.
- Optimized system monitor in Rust Agent.

### Testing and Reliability
- Added TokenService unit tests.
- Added Rust Agent validation tests.
- Improved identifier validation.

### Code Health
- Added strict TypeScript definitions.
- Added UUIDv4 validation.
