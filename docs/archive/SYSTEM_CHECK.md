# 🚦 System Requirements & Dependency Matrix

The **Rust Muscle** runs directly on the host operating system (or a privileged container) to manage jails, processes, and network routing. Before deploying Karı, the host machine must pass these pre-flight checks.



## Supported OS Matrix

| Distribution | Architecture | Support Level | Notes |
| :--- | :--- | :--- | :--- |
| **Debian 12 (Bookworm)** | x86_64, arm64 | 🟢 Tier 1 | Primary development target. |
| **Ubuntu 24.04 LTS** | x86_64, arm64 | 🟢 Tier 1 | Fully supported. |
| **Alpine Linux 3.20** | x86_64 | 🟡 Tier 2 | Requires `musl` compilation for the Rust agent. |
| **RHEL / Rocky 9** | x86_64 | 🟡 Tier 2 | SELinux policies must be manually configured for UDS access. |

## Host Dependencies

Ensure the following packages are installed on the bare-metal host or VM running the Muscle:

### 1. Core Execution
* `systemd`: Required for managing app lifecycles via `systemctl`.
* `git` (v2.30+): Required by the deployment engine to fetch source code.
* `cgroup-tools`: Required for RAM/CPU limiting of tenant applications.

### 2. Cryptography
* `openssl` (v3.0+): Required for validating and converting PEM files.
* `ca-certificates`: Required for outbound API calls.

### 3. Network Proxy (At least one)
* `nginx` or `apache2`: Karı will automatically detect the installed proxy and configure its VHosts accordingly.

## Verification Command
You can run the built-in diagnostic tool to verify the host environment before booting the cluster:
```bash
./kari-agent --health-check
```
