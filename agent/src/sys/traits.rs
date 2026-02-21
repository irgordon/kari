// agent/src/sys/traits.rs

use async_trait::async_trait;
use crate::sys::secrets::ProviderCredential;

// ==============================================================================
// 1. Firewall Abstraction
// ==============================================================================

pub struct FirewallPolicy {
    pub action: String, // ALLOW, DENY, REJECT
    pub port: u32,
    pub protocol: String, // TCP, UDP
    pub source_ip: String,
}

#[async_trait]
pub trait FirewallManager: Send + Sync {
    /// Applies a strict policy. The underlying implementation decides if this
    /// translates to `ufw allow ...` or `firewall-cmd --add-port=...`
    async fn apply_policy(&self, policy: &FirewallPolicy) -> Result<(), String>;
}

// ==============================================================================
// 2. Job Scheduler Abstraction
// ==============================================================================

pub struct JobIntent {
    pub name: String,
    pub command: String,
    pub schedule: String,
    pub user: String,
}

#[async_trait]
pub trait JobScheduler: Send + Sync {
    /// Schedules a recurring job. The underlying implementation decides if this
    /// translates to a `/etc/cron.d/` file or a `.timer` systemd unit.
    async fn schedule_job(&self, intent: &JobIntent) -> Result<(), String>;
}

// ==============================================================================
// 3. SSL Engine Abstraction
// ==============================================================================

pub struct SslPayload {
    pub domain_name: String,
    pub fullchain_pem: Vec<u8>,
    pub privkey_pem: ProviderCredential, // Wrapped in secrecy, zeroized after use
}

#[async_trait]
pub trait SslEngine: Send + Sync {
    /// Installs the certificate to the correct distro-specific paths 
    /// (e.g., /etc/ssl/certs vs /etc/pki/tls/certs)
    async fn install_certificate(&self, payload: SslPayload) -> Result<(), String>;
}
