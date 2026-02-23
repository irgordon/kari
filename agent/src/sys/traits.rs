use async_trait::async_trait;
use std::collections::HashMap;
use std::path::Path;
use tokio::sync::mpsc;
use tonic::Status;

use crate::server::kari_agent::LogChunk;
use crate::sys::secrets::ProviderCredential;

// ==============================================================================
// 1. GitOps & Source Control (Zero-Leak Auth)
// ==============================================================================

#[async_trait]
pub trait GitManager: Send + Sync {
    /// Clones a repository into a strictly typed target directory.
    /// 🛡️ Zero-Trust: ssh_key MUST be passed inside the ProviderCredential wrapper.
    /// By taking `Option<ProviderCredential>` by value, we transfer ownership to the 
    /// implementation, ensuring it is proactively zeroized the moment the clone finishes.
    async fn clone_repo(
        &self, 
        repo_url: &str, 
        branch: &str, 
        target_dir: &Path, // 🛡️ SLA: Strict Type
        ssh_key: Option<ProviderCredential> 
    ) -> Result<(), String>;
}

// ==============================================================================
// 2. Build & Execution (Telemetry-Aware)
// ==============================================================================

#[async_trait]
pub trait BuildManager: Send + Sync {
    /// Executes a build command within an unprivileged jail.
    /// 🛡️ log_tx: A streaming channel to pipe stdout/stderr back to the gRPC stream.
    async fn execute_build(
        &self,
        build_command: &str,
        working_dir: &Path, // 🛡️ SLA: Strict Type
        run_as_user: &str,
        env_vars: &HashMap<String, String>,
        log_tx: mpsc::Sender<Result<LogChunk, Status>>,
        trace_id: String,
    ) -> Result<(), String>;
}

// ==============================================================================
// 3. Firewall Abstraction (Type-Safe & Zero-Trust)
// ==============================================================================

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FirewallAction { Allow, Deny, Reject }

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Protocol { Tcp, Udp, Both }

pub struct FirewallPolicy {
    pub action: FirewallAction,
    pub port: u16,
    pub protocol: Protocol,
    pub source_ip: Option<String>,
}

#[async_trait]
pub trait FirewallManager: Send + Sync {
    async fn apply_policy(&self, policy: &FirewallPolicy) -> Result<(), String>;
}

// ==============================================================================
// 4. SSL Engine Abstraction (Memory Safe)
// ==============================================================================

pub struct SslPayload {
    pub domain_name: String,
    pub fullchain_pem: String, // PEMs are valid UTF-8, String is safer for validation than raw Vec<u8>
    
    /// 🛡️ Zero-Copy Secret. The SslEngine takes ownership of this struct,
    /// writes the key to the protected `/etc/kari/ssl` directory, and immediately
    /// calls `.destroy()` on it to scrub the RAM.
    pub privkey_pem: ProviderCredential, 
}

#[async_trait]
pub trait SslEngine: Send + Sync {
    async fn install_certificate(&self, payload: SslPayload) -> Result<(), String>;
}

// ==============================================================================
// 5. Proxy Abstraction (Platform-Agnostic Ingress)
// ==============================================================================

#[async_trait]
pub trait ProxyManager: Send + Sync {
    /// Creates a virtual host configuration for the given domain,
    /// proxying traffic to the specified internal port.
    async fn create_vhost(&self, domain: &str, target_port: u16) -> Result<(), String>;

    /// Removes the virtual host configuration for the given domain.
    async fn remove_vhost(&self, domain: &str) -> Result<(), String>;
}

// ==============================================================================
// 6. Job Scheduling Abstraction (Zero-Trust Cron)
// ==============================================================================

/// 🛡️ Zero-Trust: Discrete fields prevent shell injection via OS execve.
pub struct JobIntent {
    pub name: String,
    pub binary: String,
    pub args: Vec<String>,
    pub schedule: String,        // Systemd OnCalendar format
    pub run_as_user: String,
}

#[async_trait]
pub trait JobScheduler: Send + Sync {
    /// Schedules a recurring job using the platform's native scheduler.
    /// 🛡️ SLA: The binary + args split prevents shell interpretation.
    async fn schedule_job(&self, intent: &JobIntent) -> Result<(), String>;
}

// ==============================================================================
// 7. Release Hygiene (SLA: Disk Space Management)
// ==============================================================================

#[async_trait]
pub trait ReleaseManager: Send + Sync {
    async fn prune_old_releases(&self, releases_dir: &Path, keep_count: usize) -> Result<usize, String>;
}

// ==============================================================================
// 8. Log Management (SLA: Compliance & Rotation)
// ==============================================================================

#[async_trait]
pub trait LogManager: Send + Sync {
    async fn configure_logrotate(&self, domain_name: &str, log_dir: &str) -> Result<(), String>;
}
