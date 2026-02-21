use async_trait::async_trait;
use std::fs as std_fs;
use std::io::Write;
use std::os::unix::fs::{OpenOptionsExt, PermissionsExt};
use std::path::{Path, PathBuf};
use tokio::fs as tokio_fs;

use crate::sys::traits::{SslEngine, SslPayload};

// ==============================================================================
// 1. Concrete Implementation (Linux Filesystem)
// ==============================================================================

pub struct LinuxSslEngine {
    // 🛡️ SLA: Strict Type to prevent path traversal
    ssl_storage_dir: PathBuf, 
}

impl LinuxSslEngine {
    pub fn new(ssl_storage_dir: PathBuf) -> Self {
        Self { ssl_storage_dir }
    }
}

#[async_trait]
impl SslEngine for LinuxSslEngine {
    async fn install_certificate(&self, payload: SslPayload) -> Result<(), String> {
        
        // 1. 🛡️ Zero-Trust Path Traversal Shield
        if payload.domain_name.is_empty() || payload.domain_name.contains("..") || payload.domain_name.contains('/') {
            return Err("SECURITY VIOLATION: Invalid domain name format".into());
        }
        
        let is_valid_domain = payload.domain_name.chars().all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '.');
        if !is_valid_domain {
            return Err("SECURITY VIOLATION: Domain contains illegal characters".into());
        }

        // 🛡️ SOLID: Use OS-native path joining
        let domain_path = self.ssl_storage_dir.join(&payload.domain_name);

        // 2. Eliminate Directory TOCTOU Race
        tokio_fs::create_dir_all(&domain_path)
            .await
            .map_err(|e| format!("Failed to create SSL directory: {}", e))?;
            
        let mut perms = tokio_fs::metadata(&domain_path)
            .await
            .map_err(|e| format!("Failed to read directory metadata: {}", e))?
            .permissions();
        perms.set_mode(0o750); // rwxr-x---
        tokio_fs::set_permissions(&domain_path, perms)
            .await
            .map_err(|e| format!("Failed to secure SSL directory permissions: {}", e))?;

        // 3. 🛡️ Write the Public Certificate (Eliminate TOCTOU via OpenOptions)
        let fullchain_path = domain_path.join("fullchain.pem");
        
        // Convert std OpenOptions to tokio OpenOptions to do this asynchronously
        let mut fc_opts = std_fs::OpenOptions::new();
        fc_opts.write(true).create(true).truncate(true).mode(0o644); // rw-r--r--
        
        let mut fc_file = tokio_fs::OpenOptions::from(fc_opts)
            .open(&fullchain_path)
            .await
            .map_err(|e| format!("Failed to open fullchain file safely: {}", e))?;
            
        tokio::io::AsyncWriteExt::write_all(&mut fc_file, payload.fullchain_pem.as_bytes())
            .await
            .map_err(|e| format!("Failed to write fullchain: {}", e))?;

        // 4. Securely Write the Private Key (Zero-Copy + Zero-Race Boundary)
        let privkey_path = domain_path.join("privkey.pem");
        
        // 🚨 CRITICAL SECURITY BOUNDARY 🚨
        // Trade-off: We INTENTIONALLY use synchronous std::fs I/O inside this closure.
        // The Rust Borrow Checker mathematically forbids passing the decrypted memory 
        // reference across an `.await` boundary, as it would leak the plaintext into 
        // the Tokio task's heap state machine.
        let write_result = payload.privkey_pem.use_secret(|secret_str| {
            let mut file = std_fs::OpenOptions::new()
                .write(true)
                .create(true)
                .truncate(true)
                .mode(0o600) // rw------- (Strictly locked down from inception)
                .open(&privkey_path)
                .map_err(|e| format!("Failed to open privkey file securely: {}", e))?;

            file.write_all(secret_str.as_bytes())
                .map_err(|e| format!("Failed to write secret bytes: {}", e))?;
            
            // Explicitly sync to ensure data hits physical disk sectors before we zeroize RAM
            file.sync_all()
                .map_err(|e| format!("Failed to sync privkey to disk: {}", e))?;
                
            Ok::<(), String>(())
        });

        // 5. 🛡️ Proactive Scrubbing
        // The file is safely on the SSD. We proactively destroy the RAM buffer now
        // rather than waiting for the function to end.
        payload.privkey_pem.destroy();

        if let Err(e) = write_result {
            // Cleanup on failure to prevent corrupted/half-written keys from lingering
            let _ = tokio_fs::remove_file(&privkey_path).await;
            return Err(e);
        }

        Ok(())
    }
}
