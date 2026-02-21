# 🏛️ Karı Architecture & Security Model (v2026.4)

Karı is a platform-agnostic orchestration engine designed for high-stakes 2026 security environments. It operates on a **Deep Zero-Trust** model, ensuring that a compromise in one layer cannot escalate to host-level access.

---

## 🛠️ The Three Pillars

### 1. The Window (SvelteKit Frontend)

The "Window" is a hardened SvelteKit 5 application acting as the administrative interface.

* **Tech:** SvelteKit, Vite, Tailwind CSS, Lucide, xterm.js.
* **Edge Security:** Implements **JWT Edge Verification** via `hooks.server.ts` using the `jose` library. It strictly enforces `HttpOnly`, `SameSite=Strict` cookies.
* **Telemetry:** Utilizes **Server-Sent Events (SSE)** for real-time log streaming, providing a unidirectional, proxy-friendly pipe for build logs.
* **Deployment:** Containerized via **Node-Adapter** on a non-root Alpine base.

### 2. The Brain (Go API Gateway)

The "Brain" is the authoritative state machine. It is the only component that speaks to the Database and the outside world simultaneously.

* **Tech:** Go 1.22+, PostgreSQL 16, gRPC (Client).
* **Hardening:** Built as a **Multi-stage Distroless** image. It contains no shell, no `curl`, and no package manager, drastically reducing the RCE (Remote Code Execution) attack surface.
* **Health & SLA:** Features a custom **Go-based Healthcheck Prober** that verifies the gRPC backplane link before reporting the service as "Ready" to the UI.
* **CryptoService:** Handles **AES-256-GCM** encryption/decryption of application secrets. Plaintext secrets never touch the database.

### 3. The Muscle (Rust System Agent)

The "Muscle" is the execution engine. It is physically isolated from the internet and the Brain’s network.

* **Tech:** Rust 1.80+, gRPC (Tonic Server), systemd, cgroup v2.
* **Jail Strategy:** Uses `systemd-run` and `cgroup v2` to create transient, resource-limited jails for user applications.
* **Zero-Trust Link:** Communicates via a **Unix Domain Socket** (`/var/run/kari/agent.sock`). It enforces **gRPC Peer Credentials**—it will only accept commands if the calling process UID matches the Brain's specific ID (1001).
* **Memory Privacy:** Implements strict zeroization of buffers after sensitive private keys are used for Git operations.

---

## 🌐 Network & Communication Topology

| Link | Protocol | Security Layer |
| --- | --- | --- |
| **Admin ➜ Window** | HTTPS | TLS 1.3 + JWT |
| **Window ➜ Brain** | HTTP (Internal) | Docker Backplane + JWT |
| **Brain ➜ Muscle** | **gRPC (UDS)** | **PeerCreds (UID Validation)** |
| **Brain ➜ DB** | SQL | Scoped Credentials + Internal Bridge |

---

## 🛡️ Security Mandates

1. **Forensic Observability:** Every system event is captured in the **Action Center** and **System Logs**, with unique `trace_id` propagation from the UI down to the Rust execution logs.
2. **Immutability:** The Brain and Window filesystems are read-only in production. All persistent state is strictly offloaded to Postgres or the specific `dev_root` managed by the Muscle.
3. **Fail-Closed Design:** If the gRPC link to the Muscle is severed, the Brain immediately reports as `Unhealthy`, and the UI "locks" all deployment actions to prevent state corruption.

---

## 🏁 Development & Orchestration

The entire lifecycle is managed via a hardened `Makefile` and `docker-compose` environment, ensuring that development environments are bit-for-bit mirrors of the production security posture.

---
