// agent/src/sys/traits.rs

use async_trait::async_trait;

#[async_trait]
pub trait ReleaseManager: Send + Sync {
    /// Keeps the `keep_count` most recent releases and deletes the rest
    async fn prune_old_releases(&self, releases_dir: &str, keep_count: usize) -> Result<usize, String>;
}

#[async_trait]
pub trait LogManager: Send + Sync {
    /// Generates a logrotate configuration for a specific application
    async fn configure_logrotate(&self, domain_name: &str, log_dir: &str) -> Result<(), String>;
}
