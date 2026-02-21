// agent/src/sys/ssl.rs

use async_trait::async_trait;
use std::fs as std_fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;
use tokio::fs as tokio_fs;

use crate::sys::traits::{SslEngine, SslPayload};

// ==============================================================================
// 1. Concrete Implementation (Linux Filesystem)
// ==============================================================================

pub struct LinuxSslEngine {
    // Injected via AgentConfig at startup (e.g., "/etc/kari/ssl")
    ssl_storage_dir: String, 
}

impl LinuxSslEngine {
    pub fn new(ssl_storage_dir: String) -> Self {
        Self { ssl_storage_dir }
    }
}

#[async_trait]
impl SslEngine for LinuxSslEngine {
    async fn install_certificate(&self, payload: SslPayload) -> Result<(), String> {
        // 1. Construct the isolated path dynamically
        let domain_dir = format!("{}/{}", self.ssl_storage_dir, payload.domain_name);
        let domain_path = Path::new(&domain_dir);

        // 2. Ensure the directory exists with strict access controls
        if !domain_path.exists() {
            tokio_fs::create_dir_all(domain_path)
                .await
                .map_err(|e| format!("Failed to create SSL directory: {}", e))?;
            
            // Lock down the directory: 0o750 (rwxr-x---)
            let mut perms = tokio_fs::metadata(domain_path).await.unwrap().permissions();
            perms.set_mode(0o750);
            if let Err(e) = tokio_fs::set_permissions(domain_path, perms).await {
                return Err(format!("Failed to secure SSL directory permissions: {}", e));
            }
        }

        // 3. Write the Public Certificate (Fullchain)
        // This is safe to hold in async memory and write via Tokio
        let fullchain_path = format!("{}/fullchain.pem", domain_dir);
        tokio_fs::write(&fullchain_path, &payload.fullchain_pem)
            .await
            .map_err(|e| format!("Failed to write fullchain.pem: {}", e))?;
        
        let mut fc_perms = tokio_fs::metadata(&fullchain_path).await.unwrap().permissions();
        fc_perms.set_mode(0o644); // Publicly readable (rw-r--r--)
        let _ = tokio_fs::set_permissions(&fullchain_path, fc_perms).await;

        // 4. Securely Write the Private Key (Zero-Copy Memory Boundary)
        let privkey_path = format!("{}/privkey.pem", domain_dir);
        
        // 🚨 CRITICAL SECURITY BOUNDARY 🚨
        // We use standard synchronous `std::fs` inside the `use_secret` closure. 
        // Why? Async runtimes (Tokio) yield execution and park state in memory across threads. 
        // By using a synchronous write inside the closure, we guarantee the CPU executes the file 
        // write immediately, and the `secrecy` wrapper zeroizes the RAM the instant the closure ends.
        let write_result = payload.privkey_pem.use_secret(|secret_bytes| {
            std_fs::write(&privkey_path, secret_bytes)
        });

        if let Err(e) = write_result {
            // If the write fails, we immediately abort. The memory is still safely wiped.
            return Err(format!("Failed to securely write privkey.pem: {}", e));
        }

        // 5. Lock down the Private Key
        // Only root (the Rust Agent) and the web server group (e.g., www-data) should ever read this.
        let mut pk_perms = std_fs::metadata(&privkey_path)
            .map_err(|e| format!("Failed to read privkey metadata: {}", e))?
            .permissions();
            
        pk_perms.set_mode(0o600); // Strictly root-only (rw-------)
        if let Err(e) = std_fs::set_permissions(&privkey_path, pk_perms) {
            // If we fail to secure the file, we must delete it to prevent leakage.
            let _ = std_fs::remove_file(&privkey_path);
            return Err(format!("Failed to apply 0600 permissions to privkey.pem: {}", e));
        }

        Ok(())
    }
}
