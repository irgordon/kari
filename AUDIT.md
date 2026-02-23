# Concurrency Audit Report

## 1. Scope and Context

**Goal:** Assess the codebase for concurrency defects, with a focus on race conditions, deadlocks, data inconsistencies, livelocks, starvation, and order violations.

**Context:**
-   **Languages & Runtimes:**
    -   **Rust Agent (`agent/`):** Uses `tokio` (async/await) for concurrency, `tonic` for gRPC, and `Arc` for shared state management. Interacts heavily with the filesystem and system commands (`git`, `systemctl`).
    -   **Go API (`api/`):** Uses standard Go concurrency primitives (goroutines, channels) and `database/sql` for PostgreSQL interactions.
-   **Concurrency Model:**
    -   **Agent:** Async request handling via `tonic`. Heavy use of `tokio::spawn` for background tasks (e.g., streaming deployments). State is shared via `Arc<dyn Manager>`.
    -   **API:** Goroutine-based worker pool (currently single-threaded worker). Database locking (`FOR UPDATE SKIP LOCKED`) used for task claiming.
-   **Critical Components:**
    -   **Agent:** `KariAgentService` (gRPC entry point), `SystemGitManager`, `SystemBuildManager`, `LinuxSystemdManager`.
    -   **API:** `DeploymentWorker` (orchestrates deployments), `PostgresDeploymentRepository` (task queue).

---

## 2. Race Conditions (High Severity)

**Definition:** Two or more threads access shared state without proper synchronization, and at least one write is involved.

### Findings

1.  **Timestamp Collision in Deployment Directory (Rust Agent)**
    -   **Location:** `agent/src/server.rs:307`
    -   **Code:** `let timestamp = chrono::Utc::now().format("%Y%m%d%H%M%S").to_string();`
    -   **Issue:** The timestamp resolution is one second. If two deployment requests for the same domain arrive within the same second, they will generate the same `release_dir` path.
    -   **Consequence:** Both requests will attempt to `git clone` into the same directory. `git clone` will fail if the directory exists and is not empty, causing the second request to fail (Denial of Service). If `git` behavior varies or if `mkdir` races occur, repository corruption could happen.
    -   **Status:** 🔴 **Critical Risk**

2.  **Systemd Unit File Overwrite (Rust Agent)**
    -   **Location:** `agent/src/sys/systemd.rs:88`
    -   **Code:** `fs::write(&path, unit_content).await...`
    -   **Issue:** `tokio::fs::write` truncates and writes the file. If two concurrent requests for the same service name occur, they may interleave writes or one may overwrite the other's changes mid-deployment.
    -   **Consequence:** Corrupted systemd unit files or inconsistent service configuration.
    -   **Status:** 🟠 **Medium Risk** (Mitigated by the fact that service names are derived from domain names, which should be unique per user, but race is possible if same user triggers double deploy).

3.  **SSH Key Temp File Race (Rust Agent)**
    -   **Location:** `agent/src/sys/git.rs:44`
    -   **Code:** `NamedTempFile::new()`
    -   **Assessment:** `NamedTempFile` guarantees unique filenames via OS primitives (`O_EXCL`).
    -   **Status:** ✅ **Safe**

### Deliverable Language
> We identified a critical race condition in the deployment directory generation where second-level timestamp resolution allows collisions. We also noted potential file overwrite races in systemd unit generation. Internal memory state is largely immutable or protected by `Arc`, but filesystem shared state requires stronger isolation (e.g., UUIDs or nanosecond timestamps).

---

## 3. Deadlocks (Low Severity)

**Definition:** Two or more threads wait indefinitely for each other’s locks/resources.

### Findings

-   **Rust Agent:** The codebase uses `Arc` but very few internal mutexes in the business logic paths reviewed. Communication is primarily via `mpsc` channels (`server.rs` streaming), which are async and non-blocking (or blocking with timeout/backpressure). No cyclic lock dependencies were found.
-   **Go API:** Uses `database/sql` which handles connection pooling. `ClaimNextPending` uses `FOR UPDATE SKIP LOCKED` which avoids database-level deadlocks between workers.

### Deliverable Language
> We constructed a conceptual dependency graph and found no cycles. The architecture relies heavily on message passing (gRPC, Channels) and database-level locking (`SKIP LOCKED`) rather than fine-grained in-process mutexes, significantly reducing deadlock risk.

---

## 4. Data Inconsistencies / Corruption (Medium Severity)

**Definition:** Observing or persisting logically invalid state.

### Findings

1.  **Lack of Atomic Rollback in Deployment (Rust Agent)**
    -   **Location:** `agent/src/server.rs:328` (`stream_deployment`)
    -   **Issue:** The deployment is a multi-step process (Git Clone -> Jail -> Build -> Proxy -> Service). If a later step (e.g., Build) fails, the artifacts from earlier steps (e.g., the cloned repository on disk) are not cleaned up automatically in the error path.
    -   **Consequence:** Disk usage accumulation (orphaned `releases` directories). Use of `delete_deployment` is manual.
    -   **Status:** 🟠 **Medium Risk** (Resource leak, not data corruption).

2.  **Database Transaction Integrity (Go API)**
    -   **Location:** `api/internal/db/postgres/deployment_repository.go:27`
    -   **Code:** `ClaimNextPending` uses `Tx` correctly with `defer tx.Rollback()` and `COMMIT`.
    -   **Status:** ✅ **Safe**

### Deliverable Language
> We verified that database state transitions are atomic. However, filesystem operations in the Agent lack automatic rollback on failure, leading to potential resource leakage (orphaned directories) though not logical data corruption.

---

## 5. Livelocks (Low Severity)

**Definition:** Threads keep changing state in response to each other but make no progress.

### Findings

-   **Go API Worker:** The `DeploymentWorker` loops with a `time.Ticker` (5s). If `ClaimNextPending` returns nothing, it waits. This is a standard polling pattern and does not constitute a livelock.
-   **Retry Logic:** No tight spin-loops were observed.

### Deliverable Language
> We reviewed polling and retry loops. The `DeploymentWorker` uses a fixed interval ticker, preventing tight loops. No livelock patterns were identified.

---

## 6. Starvation (High Severity)

**Definition:** A thread or task is perpetually denied resources and never completes.

### Findings

1.  **Single Worker Bottleneck (Go API)**
    -   **Location:** `api/cmd/kari-api/main.go:126`
    -   **Code:** `go deployWorker.Start(workerCtx)` (Called once).
    -   **Issue:** Only one goroutine is processing deployments. If `stream_deployment` hangs (e.g., large repo clone, long build), all other pending deployments are blocked indefinitely.
    -   **Consequence:** Severe throughput limitation. A single slow deployment halts the entire platform's deployment capability.
    -   **Status:** 🔴 **Critical Risk**

2.  **Unbounded Concurrent Tasks (Rust Agent)**
    -   **Location:** `agent/src/server.rs:322`
    -   **Code:** `tokio::spawn(async move { ... })`
    -   **Issue:** The gRPC handler spawns a new background task for every deployment request without checking a semaphore or worker pool limit.
    -   **Consequence:** A flood of requests could exhaust system resources (file descriptors, PIDs, RAM), causing the node to crash or become unresponsive (DoS).
    -   **Status:** 🟠 **Medium Risk**

### Deliverable Language
> We identified a critical starvation risk in the API where a single worker processes all deployments sequentially. Conversely, the Agent exhibits an unbounded concurrency risk, spawning tasks without limits, which could lead to resource exhaustion under load.

---

## 7. Order Violations (Low Severity)

**Definition:** Code assumes a specific order between operations, but the concurrency model does not guarantee it.

### Findings

-   **Deployment Steps:** `stream_deployment` enforces order via `await` calls: `git.clone_repo(...).await?` then `jail.secure_directory(...).await?`. This sequential execution ensures correct ordering.
-   **Log Streaming:** Logging tasks (`stdout_task`, `stderr_task`) in `BuildManager` are joined (`tokio::join!`) before the build is considered complete, ensuring all logs are flushed.

### Deliverable Language
> We verified that all critical sequences (deployment steps, log flushing) are enforced via explicit `await` points and synchronization barriers (`tokio::join!`).

---

## 8. Recommended Fixes

1.  **Fix Timestamp Collision:**
    -   **Change:** Use `uuid::Uuid::new_v4()` or include nanoseconds in the folder name.
    -   **File:** `agent/src/server.rs`

2.  **Increase Worker Count:**
    -   **Change:** Instantiate a pool of `DeploymentWorker`s or make `processNextTask` run in a separate goroutine (with a bounded semaphore).
    -   **File:** `api/cmd/kari-api/main.go` or `api/internal/worker/deployment_worker.go`

3.  **Implement Bounded Concurrency in Agent:**
    -   **Change:** Use a `Semaphore` to limit the number of concurrent deployment tasks.
    -   **File:** `agent/src/server.rs`

4.  **Add Automatic Cleanup:**
    -   **Change:** Implement a "Rollback" struct or `defer`-like pattern in Rust that deletes the `release_dir` if the function returns an error.
    -   **File:** `agent/src/server.rs`
