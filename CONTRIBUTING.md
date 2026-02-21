# 🛠️ Contributing to Karı

Thank you for contributing! To maintain our **Zero-Trust** and **Platform-Agnostic** nature, every Pull Request is evaluated against these four pillars.

## 1. The Golden Rule: No Hardcoded Paths

Karı must run on any Linux distribution (Ubuntu, Arch, Alpine, FreeBSD). Hardcoding paths like `/etc/nginx` or `/usr/bin/python3` is strictly prohibited.

* **Go Brain:** Inject paths via the `config.Config` struct, sourced from environment variables.
* **Rust Muscle:** Use the `AgentConfig` struct. If a path is needed, it must be provided by the Brain via a gRPC message or a configuration flag.
* **The Check:** If your code contains a string starting with `/` (outside of a configuration default), it will be rejected.

## 2. Environment Agnosticism

Karı should not know if it is running in a Docker container, a Virtual Private Server, or a Raspberry Pi.

* **Use Abstract Intents:** Never assume a specific package manager (like `apt`). The Go Brain sends an "Intent" (e.g., `InstallPackage{name: "nginx"}`), and the Rust Agent determines if it should use `pacman`, `dnf`, or `apt`.
* **Feature Detection:** Instead of checking for an OS version, check for the presence of a capability (e.g., "Does `systemctl` exist?").

## 3. SOLID & SLA (Single Layer Abstraction)

Every component must follow the **Single Responsibility Principle**.

* **The Brain (Go) is the Policy:** It handles RBAC, validation, and "The What." It never touches the disk or runs a shell command.
* **The Muscle (Rust) is the Execution:** It handles "The How." It implements traits defined in `sys/traits.rs`.
* **SLA Enforcement:**
* **Logic belongs in Services:** Handlers should only parse input and call a Service.
* **Data belongs in Repositories:** Services should never write raw SQL.
* **System calls belong in Traits:** The gRPC server calls an `SslEngine` trait, not a specific "LinuxSslWriter."



## 4. Secure Execution (Zero Shell Policy)

We bypass `sh`, `bash`, and `zsh` entirely to prevent command injection.

* **Rust:** Always use `std::process::Command::new("binary").arg("param")`. Never use `.arg(format!("..."))` without strict white-listing.
* **Privilege Dropping:** Any command that interacts with user-supplied code (like `npm install`) must utilize the `jail.rs` module to drop privileges to the tenant's specific unprivileged user.

## 5. Memory-Safe Secret Handling

* All private keys and sensitive tokens in Rust must be wrapped in the `secrecy` crate.
* Ensure the `Zeroize` trait is implemented or called for any buffer holding plaintext credentials.
* **Standard:** Sensitive data should exist in plaintext in RAM for the shortest possible duration.

## 6. PR Checklist

Before submitting a PR, ensure:

1. [ ] **`go vet`** and **`cargo clippy`** pass without warnings.
2. [ ] No new `/` paths are hardcoded.
3. [ ] New gRPC messages are defined in `proto/` first.
4. [ ] Audit logs are generated for any system mutation.
5. [ ] **Action Center** alerts are created for any recoverable failure.


When you return to your **Pro tier**, would you like me to create a **GitHub Action** that automatically scans PRs for hardcoded paths to enforce this `CONTRIBUTING.md` via CI?
