# 🏛️ Karı Architecture & Security Model

Karı is a platform-agnostic orchestration engine designed for the 2026 security landscape. It operates on a strict **Zero-Trust** model, physically and logically separating the user interface, the business logic, and the system-level execution.



## The Three Pillars

### 1. The Window (SvelteKit Frontend)
The user-facing dashboard. It is completely stateless and handles no cryptography other than storing secure `HttpOnly` cookies.
* **Tech:** SvelteKit, Vite, Tailwind CSS, Node.js.
* **Role:** UI rendering, granular RBAC evaluation, and proxying user intent to the Brain.
* **Security:** Enforces CSRF protections and drops all debug logs via `esbuild`. Runs as a restricted non-root user in Docker.

### 2. The Brain (Go API Gateway)
The authoritative orchestrator. It holds the database connections, issues JWTs, and validates all business logic.
* **Tech:** Go 1.22+, PostgreSQL 16.
* **Role:** Authentication (Stateless JWT + Silent Refresh), API routing, Let's Encrypt (ACME) orchestration, and state persistence.
* **Security:** Never touches the host filesystem. All system changes are delegated to the Muscle over a secure gRPC pipe.

### 3. The Muscle (Rust System Agent)
The execution engine. This is the only component that runs with elevated privileges on the host machine.
* **Tech:** Rust 1.80+, gRPC (Tonic), systemd.
* **Role:** Managing Linux namespaces/cgroups, restarting reverse proxies (Nginx/Apache), writing SSL certificates, and managing Git clones.
* **Security:** Communicates with the Brain *exclusively* via a shared Unix Domain Socket (`/var/run/kari/agent.sock`). Zero network exposure. Zeroizes memory after handling sensitive private keys.

## Network Topology
* **Frontend Network:** Exposed to the internet via standard ports (80/443).
* **Backplane Network:** Strictly internal Docker bridge. The PostgreSQL database and the Rust Muscle have **no public internet ingress**.
