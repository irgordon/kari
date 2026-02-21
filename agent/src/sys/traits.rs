// agent/src/sys/traits.rs

use async_trait::async_trait;
use std::net::IpAddr;
use crate::sys::secrets::ProviderCredential;

// ==============================================================================
// 1. Firewall Abstraction (Type-Safe & Zero-Trust)
// ==============================================================================

/// 🛡️ SLA Enforcement: Mathematical bounds on allowed firewall actions.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FirewallAction {
    Allow,
    Deny,
    Reject,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Protocol {
    Tcp,
    Udp,
    Both,
}

pub struct FirewallPolicy {
    pub action: FirewallAction,
    pub port: u16,              // u16 is the mathematical bound for TCP/UDP ports (0-65535)
    pub protocol: Protocol,
    pub source_ip: Option<IpAddr>, // Uses native Rust IP validation, None implies 'Any'
}

#[async_trait]
pub trait FirewallManager: Send + Sync {
    /// Applies a strict, type-safe policy. The underlying implementation decides if this
    /// translates to `ufw allow ...` or `firewall-cmd --add-port=...`
    async fn apply_policy(&self, policy: &FirewallPolicy) -> Result<(), String>;
}

// ==============================================================================
// 2. Job Scheduler Abstraction (Anti-Injection)
// ==============================================================================

pub struct JobIntent {
    pub name: String,
    // 🛡️ SLA Enforcement: We split the binary from the arguments to completely 
    // neuter shell interpolation vulnerabilities in the underlying implementation.
    pub binary: String,
    pub args: Vec<String>,
    pub schedule: String, // e.g., "0 4 * * *"
    pub run_as_user: String,
}

#[async_trait]
pub trait JobScheduler: Send + Sync {
    /// Schedules a recurring job. The underlying implementation decides if this
    /// translates to a `/etc/cron.d/` file or a `.timer` systemd unit.
    async fn schedule_job(&self, intent: &JobIntent) -> Result<(), String>;
}

// ==============================================================================
// 3. SSL Engine Abstraction (Memory Safe)
// ==============================================================================

pub struct SslPayload {
    pub domain_name: String,
    pub fullchain_pem: Vec<u8>,
    // 🛡️ Zero-Copy Secret. The memory will be physically zeroized the moment 
    // the `install_certificate` function finishes execution.
    pub privkey_pem: ProviderCredential, 
}

#[async_trait]
pub trait SslEngine: Send + Sync {
    /// Installs the certificate to the correct distro-specific paths 
    /// (e.g., /etc/ssl/certs vs /etc/pki/tls/certs)
    async fn install_certificate(&self, payload: SslPayload) -> Result<(), String>;
}
