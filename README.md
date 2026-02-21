<div align="center">
  <img src="kari-logo.png" alt="Kari Logo" width="240">

  <h1>Karı — Made Simple. Designed Secure. </h1>
  <p>A fast, friendly control panel that installs in minutes and makes server management effortless, safe, and actually enjoyable. Get powerful tools, a clean interface, and complete control without the clutter.</p>

  <p>
    <img src="https://img.shields.io/badge/go-%2300ADD8.svg?style=for-the-badge&logo=go&logoColor=white" alt="Go" />
    <img src="https://img.shields.io/badge/rust-%23000000.svg?style=for-the-badge&logo=rust&logoColor=white" alt="Rust" />
    <img src="https://img.shields.io/badge/svelte-%23f1413d.svg?style=for-the-badge&logo=svelte&logoColor=white" alt="Svelte" />
    <img src="https://img.shields.io/badge/postgres-%23316192.svg?style=for-the-badge&logo=postgresql&logoColor=white" alt="PostgreSQL" />
    <img src="https://img.shields.io/badge/nginx-%23009639.svg?style=for-the-badge&logo=nginx&logoColor=white" alt="Nginx" />
    <img src="https://img.shields.io/badge/gRPC-%23244c5a.svg?style=for-the-badge&logo=grpc&logoColor=white" alt="gRPC" />
    <img src="https://img.shields.io/badge/GitHub_Actions-2088FF?style=for-the-badge&logo=github-actions&logoColor=white" alt="GitHub Actions" />
    <img src="https://img.shields.io/badge/license-MIT-blue.svg?style=for-the-badge" alt="MIT License" />
  </p>
</div>
---

Karı is a next-generation server control panel built for the workflows of 2026 and beyond. Designed to replace legacy monolithic panels, Karı brings the seamless, GitOps-driven developer experience of platforms like Vercel or Railway directly to your own infrastructure.

Built with an unprivileged **Go** REST API and a memory-safe, root-level **Rust** system agent, Karı acts as a **Platform-Agnostic Orchestration Engine**, offering blisteringly fast performance and an impenetrable security boundary.

## ✨ Core Features

* **Platform-Agnostic Orchestration:** The Go API dictates *intent* (Policies), while the Rust Agent handles OS-specific *execution* (Rules). Easily portable across Ubuntu, Debian, AlmaLinux, and Fedora.
* **GitOps by Default:** Native webhooks for GitHub/GitLab validated via constant-time HMAC SHA-256. Push to `main`, and Karı clones, builds, and swaps your app with zero-downtime atomic symlinks.
* **Systemd User Jails:** First-class support for Node.js, Python, PHP, and Ruby. Apps run isolated under unprivileged system users with strict cgroup quotas, `ProtectSystem=full`, and `PrivateTmp=true`.
* **Zero-Copy Secrets:** Provider tokens and TLS private keys are wrapped in Rust `secrecy` and `zeroize` memory guards. Keys are physically overwritten in RAM the millisecond they are no longer needed.
* **Automated Auto-Renewing SSL:** Native Let's Encrypt integration. Certificates are provisioned securely in memory, written directly to root-owned files, and auto-renewed by a background Go worker.
* **Dynamic RBAC & Action Center:** Rank-based permissions prevent privilege escalation. High-performance observability via GIN-indexed JSONB system alerts.
* **Real-Time Observability:** End-to-end WebSockets stream deployment build logs directly to an XSS-proof `xterm.js` terminal UI in real-time.

---

## 🏗️ Architecture

Karı uses a strict privilege-separation model, splitting operations across three distinct boundaries to ensure absolute security and high performance. Data dictates behavior; there are zero hardcoded system paths or business defaults.

```mermaid
graph TD
    %% Styling
    classDef frontend fill:#ff3e00,stroke:#fff,stroke-width:2px,color:#fff;
    classDef backend fill:#00add8,stroke:#fff,stroke-width:2px,color:#fff;
    classDef agent fill:#000000,stroke:#fff,stroke-width:2px,color:#fff;
    classDef database fill:#336791,stroke:#fff,stroke-width:2px,color:#fff;
    classDef os fill:#444444,stroke:#fff,stroke-width:2px,color:#fff;

    %% Nodes
    subgraph "The UI Layer"
        UI["💻 Kari Web (SvelteKit SPA)<br/>Action Center, GitOps UI, RBAC Builder"]:::frontend
    end

    subgraph "The Brain (Unprivileged Orchestrator)"
        API["🧠 Kari API (Go Daemon)<br/>RBAC, Let's Encrypt, Webhooks, Rate Limiting"]:::backend
        DB[("🗄️ PostgreSQL<br/>Encrypted Secrets, Audit Logs, State")]:::database
    end

    subgraph "The Muscle (Root Executor)"
        AGENT["⚙️ Kari Agent (Rust Daemon)<br/>Systemd Jails, Atomic Swaps, Zero-Copy Secrets"]:::agent
    end

    subgraph "The Host Environment"
        OS["🐧 Linux OS<br/>Nginx, App Runtimes, CGroup Quotas"]:::os
    end

    %% Connections
    UI <-->|"HTTPS REST & WSS Streams"| API
    API <-->|"SQL / JSONB / GIN Indices"| DB
    API <-->|"gRPC (Unix Socket / SO_PEERCRED Auth)"| AGENT
    AGENT -->|"Abstract Traits & Safe Exec"| OS

```

---

## 📂 Monorepo File Structure

```markdown
kari/
├── .github/workflows/          # CI/CD pipelines (Go build, Rust cross-compile, Svelte build)
├── agent/                      # The Muscle (Rust gRPC Daemon)
│   ├── Cargo.toml              
│   └── src/
│       ├── main.rs             # Entrypoint, secure Unix socket binding (SO_PEERCRED)
│       ├── config.rs           # Environment-injected dynamic paths (No hardcoded paths)
│       ├── server.rs           # gRPC SystemAgent implementation with Backpressure
│       └── sys/                # System Integrations (SOLID SLAs)
│           ├── traits.rs       # Abstract interfaces (JobScheduler, FirewallManager, SslEngine)
│           ├── secrets.rs      # Zeroize/Secrecy memory wrappers for credentials
│           ├── scheduler.rs    # systemd timer concrete implementation
│           ├── jail.rs         # Linux user creation and filesystem lockdown
│           ├── git.rs          # Secure Git cloning with credential scrubbing
│           └── systemd.rs      # Generates secure systemd unit files (ProtectSystem=full)
├── api/                        # The Brain (Go REST API)
│   ├── cmd/kari-api/main.go    # App entrypoint (wires dependencies, starts workers/router)
│   ├── internal/
│   │   ├── config/config.go    # Centralized configuration (No hardcoded paths)
│   │   ├── adapters/           # Concrete implementations (Nginx, ACME Provider)
│   │   ├── api/                # HTTP Transport Layer (RBAC Middleware, Handlers, Router)
│   │   ├── core/               # Business Logic (SOLID)
│   │   │   ├── domain/         # Structs, Profile configs, & Repository Interfaces
│   │   │   └── services/       # Orchestrators (Auth, App, SSL, Audit)
│   │   ├── db/                 # PostgreSQL migrations (GIN Indices) and Repositories
│   │   ├── workers/            # Background cron jobs (SSL Renewer, App Monitor)
│   │   └── grpc/               # Generated Go gRPC client (from proto)
├── frontend/                   # The UI (SvelteKit SPA)
│   ├── package.json
│   ├── tailwind.config.ts      # Brand palette and typography mapping
│   └── src/
│       ├── hooks.server.ts     # Server-side JWT gatekeeper, silent refresh logic
│       ├── lib/                # Shared UI utilities and components
│       │   ├── api/            # Frontend SLA Layer (client.ts, terminalStream.ts)
│       │   └── components/     # UI Components (Terminal.svelte, ActionCenter.svelte)
│       └── routes/             # Filesystem Routing
│           ├── (app)/          # Authenticated routes & +layout.svelte shell
│           └── (auth)/         # XSS-proof form actions
├── proto/                      # The Contract
│   └── kari/agent/v1/agent.proto # Abstract intent definitions (FirewallPolicy, JobIntent)
├── scripts/                    # DevOps & DX
└── docker-compose.yml          

```

---

## 🚀 Quick Install

To install Karı on a fresh Linux server, run our idempotent bootstrap script as `root`. This will handle OS detection, dependency bootstrapping, and security sandboxing automatically.

```bash
curl -sSL https://raw.githubusercontent.com/irgordon/kari/main/scripts/install.sh | sudo bash

```

*(Supports Ubuntu 22.04/24.04, Debian 12, AlmaLinux 9, and Fedora)*

---

## 🛠️ Local Development

### Prerequisites

* Go 1.22+
* Rust (Stable) + Cargo
* Node.js 20+
* PostgreSQL 16+
* Protocol Buffers Compiler (`protoc`)

### Getting Started

1. **Clone the repository:**

```bash
git clone https://github.com/irgordon/kari.git
cd kari

```

2. **Generate the gRPC Protobufs:**

```bash
make proto-gen

```

3. **Start the development services:**

```bash
./scripts/dev.sh

```

---

## 🛡️ Security

Security is the foundational principle of Karı. We utilize a strict two-token JWT architecture (HttpOnly cookies for the browser UI, and Personal Access Tokens for CLI usage), AES-256-GCM encryption for database secrets, and memory-safe Rust execution with proactive RAM zeroization.

If you discover a security vulnerability, please do **NOT** open a public issue. Email `security@kariapp.dev` directly.

---

## 📄 License

This project is licensed under the **[MIT License](https://mit-license.org/)**.

© 2026 Karı Project - *Made Simple. Designed Secure.*

---
