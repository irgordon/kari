# 🌍 External Providers & Integrations

Karı is designed to be **Platform-Agnostic**. Rather than hardcoding specific vendors into the core orchestration engine, we use an interface-driven provider model.

## Supported Provider Types

| Category | Role | Current Native Integrations | Planned (2027) |
| :--- | :--- | :--- | :--- |
| **ACME (SSL)** | Certificate Authority for HTTP-01 challenges. | Let's Encrypt (via `go-acme/lego`) | ZeroSSL, Custom Internal CA |
| **Reverse Proxy** | Handles incoming web traffic and routing. | Nginx, Apache | Caddy, Traefik |
| **Storage** | Offsite backups for database and app volumes. | Local Filesystem | AWS S3, Cloudflare R2 |
| **DNS** | (Optional) Automated domain propagation. | Manual A-Record config | Cloudflare, AWS Route53 |

## The `Provider` Abstraction (SLA)

To add a new reverse proxy, developers do not touch the Go Brain. They simply implement the `ChallengeProvider` trait in the Rust Muscle.

```rust
// Example concept for adding a new proxy provider in the Rust Muscle
pub trait ProxyManager {
    fn write_vhost(&self, domain: &str, port: u16) -> Result<(), AppError>;
    fn reload_service(&self) -> Result<(), AppError>;
}
```
