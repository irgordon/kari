<div align="center">
  <img src="kari-logo.png" alt="Kari Logo" width="240">

  <h1>Karı — Made Simple. Designed Secure. </h1>
  <p>A fast, friendly control panel that installs in minutes and makes server management effortless, safe, and actually enjoyable. Get powerful tools, a clean interface, and complete control without the clutter.</p>

  <p>
    <a href="https://github.com/irgordon/kari/actions"><img src="https://img.shields.io/badge/build-passing-brightgreen" alt="Build Status"></a>
    <a href="https://github.com/irgordon/kari/releases"><img src="https://img.shields.io/badge/release-v1.0.0-blue" alt="Release"></a>
    <a href="https://github.com/irgordon/kari/blob/main/LICENSE"><img src="https://img.shields.io/badge/license-MIT-purple" alt="License"></a>
  </p>

  <p>
    <img src="https://img.shields.io/badge/go-%2300ADD8.svg?style=for-the-badge&logo=go&logoColor=white" alt="Go" />
    <img src="https://img.shields.io/badge/rust-%23000000.svg?style=for-the-badge&logo=rust&logoColor=white" alt="Rust" />
    <img src="https://img.shields.io/badge/react-%2320232a.svg?style=for-the-badge&logo=react&logoColor=%2361DAFB" alt="React" />
    <img src="https://img.shields.io/badge/postgres-%23316192.svg?style=for-the-badge&logo=postgresql&logoColor=white" alt="PostgreSQL" />
    <img src="https://img.shields.io/badge/nginx-%23009639.svg?style=for-the-badge&logo=nginx&logoColor=white" alt="Nginx" />
    <img src="https://img.shields.io/badge/gRPC-%23244c5a.svg?style=for-the-badge&logo=grpc&logoColor=white" alt="gRPC" />
    <img src="https://img.shields.io/badge/GitHub_Actions-2088FF?style=for-the-badge&logo=github-actions&logoColor=white" alt="GitHub Actions" />
  </p>
</div>

---

Karı is a next-generation server control panel built for the strict security and performance demands of today's computing environment. Designed to replace legacy, vulnerable monolithic panels, Karı brings the seamless developer experience of platforms like Vercel or Railway directly to your own infrastructure.

Built with an unprivileged **Go** REST API and a memory-safe, root-level **Rust** system agent communicating exclusively over an isolated Unix Domain Socket, Karı acts as a **Platform-Agnostic Orchestration Engine**, offering blisteringly fast performance and an impenetrable security boundary.

## ✨ Core Features

- **Platform-Agnostic Orchestration:** The Go API dictates _intent_ (Policies), while the Rust Agent handles OS-specific _execution_ (Rules). Easily portable across Ubuntu, Debian, AlmaLinux, and Fedora without altering core business logic.
- **Air-Gapped Execution & Setup Lock:** The Go Brain orchestrator runs entirely unprivileged. The initialization API is physically sealed by a `setup.lock` once the cluster is provisioned, preventing unauthorized routing.
- **Zero-Trust Systemd Jails:** First-class support for Node.js, Python, PHP, and Ruby. Tenant applications run completely isolated under unprivileged system users with strict cgroup quotas, `ProtectSystem=strict`, and `RestrictSUIDSGID=true`.
- **Cryptographic Memory Guards & AAD:** Secrets are encrypted in Postgres using AES-256-GCM with Associated Authenticated Data (AAD) bound to the AppID. In the Rust agent, sensitive keys are purged from RAM instantly via `Zeroize`.
- **Dynamic RBAC & Action Center:** Cryptographically signed, stateless JWTs enforce rank-based permissions at the edge. The Go router enforces Defense-in-Depth, intrinsically rejecting mutating methods lacking proper payload scopes.
- **Real-Time Observability (SLA Compliant):** End-to-end SSE streams deployment build logs directly from the Linux host to an XSS-proof `xterm.js` terminal UI. Strict backpressure and context propagation (`kill_on_drop`) guarantee the orchestrator never blocks.
- **Premium Glass-Bento UI:** A progressively enhanced, frictionless React dashboard using translucency, "Accent Glows", and bento-grid metric visualizations for an elite operational experience.

---

<p>
  <a href="#-the-architecture">Architecture</a> •
  <a href="#-platform-agnosticism">Platform Agnosticism</a> •
  <a href="#-installation">Installation</a> •
  <a href="#-documentation">Documentation</a> •
  <a href="#-contributing">Contributing</a>
</p>

---

## 🏗️ Architecture

Karı operates on a strict **Zero-Trust** model, physically and logically separating the user interface, the business logic, and the system-level execution. It is built on three highly optimized pillars:

1. **The Window (React & Node.js):** A progressively enhanced, client-side frontend utilizing our modern **Glass-Bento** design system. It handles stateful UI rendering and granular Role-Based Access Control (RBAC) via cryptographically verified local JWTs.
2. **The Brain (Go & PostgreSQL):** The stateless API gateway. It handles authentication, AAD-backed database persistence, Let's Encrypt orchestration, and strict SLA enforcement (backpressure, DB pooling). It never touches the host OS directly.
3. **The Muscle (Rust):** The execution engine. Running as a privileged daemon, it receives gRPC commands from the Brain over a secure, isolated Unix Domain Socket (UDS) with strict PeerCred verification. It aggressively zeroizes memory after handling keys.

```mermaid
graph TD
    %% Styling
    classDef frontend fill:#ff3e00,stroke:#fff,stroke-width:2px,color:#fff;
    classDef backend fill:#00add8,stroke:#fff,stroke-width:2px,color:#fff;
    classDef agent fill:#000000,stroke:#fff,stroke-width:2px,color:#fff;
    classDef database fill:#336791,stroke:#fff,stroke-width:2px,color:#fff;
    classDef os fill:#444444,stroke:#fff,stroke-width:2px,color:#fff;

    %% Nodes
    subgraph "The Window (Edge Gatekeeper)"
        UI["💻 React UI (Glass-Bento UI)<br/>JWT Cryptographic Verification, RBAC UI"]:::frontend
    end

    subgraph "The Brain (Unprivileged Orchestrator)"
        API["🧠 Go API Gateway<br/>Stateless Auth, Let's Encrypt, Intent Routing, Webhooks"]:::backend
        DB[("🗄️ PostgreSQL 16<br/>AES-256 Encrypted Secrets, JSONB Audit Logs, GIN Indices")]:::database
    end

    subgraph "The Muscle (Privileged Executor)"
        AGENT["⚙️ Rust System Agent<br/>Systemd Jails, Atomic Symlinks, RAM Zeroization, UDS PeerCred"]:::agent
    end

    subgraph "The Host Infrastructure"
        OS["🐧 Platform-Agnostic OS<br/>Namespaces, Cgroups, Reverse Proxies (Nginx/Apache)"]:::os
    end

    %% Connections
    UI <-->|"Internal Network (brainFetch) + WSS Streaming"| API
    API <-->|"Zero-Trust SQL (kari_admin)"| DB
    API <-->|"Strict gRPC over isolated Unix Domain Socket (UDS)"| AGENT
    AGENT -->|"Abstract SLA Traits & Safe Execution"| OS

```

---

## 📂 Monorepo File Structure

```markdown
kari/
├── .github/
│ └── workflows/
│ └── release.yml # CI/CD: Cross-compiles Rust (via cross), builds Go binaries, pushes multi-arch GHCR Docker images
├── docs/ # Architectural Source of Truth
│ ├── ARCHITECTURE.md # 3-tier Zero-Trust model explanation
│ ├── AGENT_API.md # gRPC protocol buffer schema documentation
│ ├── PROVIDERS.md # Plugin/Adapter interfaces (DNS, Let's Encrypt, Nginx)
│ └── SYSTEM_CHECK.md # Linux host requirements and dependency matrix
├── agent/ # The Muscle (Rust gRPC Daemon - Privileged Execution)
│ ├── Dockerfile # Distroless/Alpine runtime for containerized mode
│ ├── Cargo.toml # Dependencies: tonic (gRPC), tokio, zeroize, openssl
│ └── src/
│ ├── main.rs # Entrypoint, secure Unix Domain Socket (UDS) binding
│ ├── config.rs # Environment-injected dynamic paths (SLA compliance)
│ ├── server.rs # gRPC SystemAgent implementation routing intents to sys layer
│ └── sys/ # System Integrations (Linux OS manipulation)
│ ├── traits.rs # Abstract interfaces for testing (ProxyManager, SslEngine)
│ ├── secrets.rs # Memory wrappers (Zeroize) to securely wipe PEM/Key buffers
│ ├── jail.rs # Linux namespace/cgroup management and user isolation
│ └── systemd.rs # Generates hardened systemd units (ProtectSystem=strict)
├── api/ # The Brain (Go REST API - Stateless Orchestrator)
│ ├── Dockerfile # Multi-stage scratch/alpine build (CGO_ENABLED=0)
│ ├── go.mod
│ ├── cmd/kari-api/main.go # App entrypoint (wires strict DB pool, Setup Guard)
│ └── internal/
│ ├── config/config.go # Environment variable ingestion
│ ├── core/ # Business Logic (SOLID)
│ │ ├── domain/ # Structs (User, App), Enums, & Repository Interfaces
│ │ └── services/ # Orchestrators (Backpressure SLA logic, AES-GCM Encyption)
│ ├── db/ # PostgreSQL implementation
│ │ └── postgres/pool.go # Optimized pgxpool for SLA concurrency guarantees
│ ├── handlers/ # HTTP Handlers (REST endpoints, Setup Wizard)
│ │ └── setup_handler.go # Transient tokens and BIP-39 recovery phrases
│ └── grpc/ # Generated Go gRPC client code
├── frontend/ # The Window (React UI - Glass-Bento Aesthetic)
│ ├── Dockerfile # Multi-stage build
│ ├── package.json
│ ├── vite.config.ts
│ ├── tailwind.config.ts # Configures Glass-Bento styles and Accent Glows
│ └── src/
│ ├── main.tsx # App entrypoint (React root mount)
│ ├── components/ # UI abstractions (Dashboard Grid, Terminals)
│ └── pages/
│ ├── Dashboard.tsx # Dashboard metrics and bento-grid layout
│ ├── Apps.tsx # Application management views
│ └── Login.tsx # Auth forms with JWT state retention
├── proto/ # The Contract (Language-Agnostic Schema)
│ └── kari/v1/agent.proto # Protocol Buffer definitions for UDS communication
├── dev.sh # Developer DX: Host-level routing, mock paths, Vite HMR
├── up.sh # Integration DX: Docker Compose stack builder and logger
├── install.sh # Bare-Metal DX: systemd sandboxing, umask 027, rootless users
└── docker-compose.yml # Production-grade isolated network, UDS volume sharing   
```

---

## 🚀 Quick Install (Bare-Metal)

To install Karı on a fresh Linux server, run our idempotent bootstrap script as `root`. This handles OS detection, dependency bootstrapping, strict directory permissioning (`umask 027`), and kernel-level systemd sandboxing automatically.

```bash
curl -sSL https://raw.githubusercontent.com/irgordon/kari/main/install.sh | sudo bash

```

## 🌍 Platform Agnosticism

Karı doesn't care where it runs. It is designed to abstract away the underlying infrastructure so you can deploy your applications seamlessly across:

- **Bare-Metal Servers & VMs** (Debian, Ubuntu, RHEL)
- **ARM Clusters** (AWS Graviton, Raspberry Pi)
- **Containerized Environments** (Docker Compose swarms)

By implementing the Single Layer Abstraction (SLA) principle, adding a new reverse proxy (e.g., swapping Nginx for Caddy) or a new DNS provider requires zero changes to the Go Brain or the React UI. You simply drop a new provider interface into the Rust Muscle.

---

## 🛠️ Local Development

Our developer experience is engineered for speed and determinism. We provide two distinct workflows depending on your testing needs.

### Safe Defaults

- No custom config is required for normal local runs.
- `make dev` is the recommended single-command validation surface.
- Protobuf tooling is optional unless you are changing `.proto` definitions.
- Advanced security/governance controls remain available but are not required for first use.

### Prerequisites

- Docker & Docker Compose (v2+)
- Go 1.22+
- Rust (Stable) + Cargo
- Node.js 20+
- Protocol Buffers Compiler (`protoc`)
- `protoc-gen-go` and `protoc-gen-go-grpc` (for Go gRPC stub generation)

### Getting Started

1. **Clone the repository:**

```bash
git clone https://github.com/irgordon/kari.git
cd kari

```

2. **Optional first-run bootstrap (one command):**

```bash
./scripts/bootstrap.sh

```

3. **Generate the gRPC Protobufs (only if modifying `.proto` contracts):**

```bash
make proto

```

4. **Run the single-command preflight (recommended):**

```bash
make proto

```

3. **Run the unified preflight checks (recommended before PRs):**

```bash
make verify

```

4. **Choose your Execution Model:**

**Option A: Fast Iteration (Native Host)**
Spins up PostgreSQL via Docker, but runs the Go Brain, Rust Muscle, and React UI natively on your machine with mocked filesystem paths (no `sudo` required). Perfect for UI work and instant Hot Module Replacement (HMR).

```bash
./dev.sh

```

**Option B: Full Integration (Containerized)**
Compiles the optimized, multi-stage Dockerfiles and boots the entire Zero-Trust architecture inside an isolated Docker Compose network. Identical to a containerized production deployment.

```bash
./up.sh

```

---

## 🛡️ Security & Zero-Trust Architecture

Security is the foundational principle of Karı. We do not trust the network, we do not trust the user, and our internal services do not even trust each other.

- **Air-Gapped Execution:** The Go Brain handles web traffic as an unprivileged, restricted user (`NoNewPrivileges=true`, empty `CapabilityBoundingSet`). It cannot touch the host OS.
- **Hermetic gRPC:** The Go Brain and the Rust Muscle communicate _exclusively_ over a local Unix Domain Socket (UDS). There is zero internal network exposure for the execution engine.
- **Systemd Sandboxing:** On bare-metal deployments, the API orchestrator is locked inside a `ProtectSystem=strict` sandbox, rendering the entire host filesystem read-only.
- **Memory Safety & Cryptography:** We utilize memory-safe Rust execution with proactive RAM zeroization (`zeroize` crate) for all private keys, AES-256-GCM encryption for database secrets, and a strict two-token JWT architecture (HttpOnly cookies for the browser UI, and Personal Access Tokens for CLI usage).

If you discover a security vulnerability, please do **NOT** open a public issue. Email `security@kariapp.dev` directly.

---

## 🛠️ Local Development

Building Karı locally is designed to be frictionless. Our local bootstrapper spins up the database in Docker, mocks the Linux filesystem so the Rust agent can run without `sudo`, and hot-reloads the React UI.

```bash
./dev.sh

```

---

## 📚 Documentation

Dive deeper into the engineering principles behind Karı:

- [Architecture & Security Model](https://www.google.com/search?q=docs/ARCHITECTURE.md)
- [The Muscle API (gRPC Schema)](https://www.google.com/search?q=docs/AGENT_API.md)
- [Provider Integrations](https://www.google.com/search?q=docs/PROVIDERS.md)
- [System Requirements & Pre-flight Checks](https://www.google.com/search?q=docs/SYSTEM_CHECK.md)
- [Quickstart: Local to Production](https://www.google.com/search?q=docs/QUICKSTART_LOCAL_PRODUCTION.md)

---

## 🤝 Contributing

We welcome contributions! Please review our [Contributing Guidelines](https://www.google.com/search?q=CONTRIBUTING.md) before submitting pull requests. Ensure your code complies with our strict Zero-Trust and SLA principles.

## 📄 License

This project is licensed under the **[MIT License](https://mit-license.org/)**.

© 2026 Karı Project - _Made Simple. Designed Secure._

---
