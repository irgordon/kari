// agent/src/sys/logs.rs

use crate::sys::traits::LogManager;
use async_trait::async_trait;
use std::os::unix::fs::PermissionsExt;
use tokio::fs as tokio_fs;

pub struct LinuxLogManager {
    logrotate_dir: String, // Injected path (e.g., "/etc/logrotate.d")
}

impl LinuxLogManager {
    pub fn new(logrotate_dir: String) -> Self {
        Self { logrotate_dir }
    }
}

#[async_trait]
impl LogManager for LinuxLogManager {
    async fn configure_logrotate(&self, domain_name: &str, log_dir: &str) -> Result<(), String> {
        // 🛡️ 1. Zero-Trust Path Traversal Shield
        // Enforce strict domain name characters to prevent directory escape.
        if domain_name.is_empty() || domain_name.contains("..") || domain_name.contains('/') {
            return Err("SECURITY VIOLATION: Invalid domain name format".into());
        }
        if !domain_name
            .chars()
            .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '.')
        {
            return Err("SECURITY VIOLATION: Domain contains illegal characters".into());
        }

        // 🛡️ 2. Prevent Config Injection
        // Ensure the log_dir doesn't contain characters that could break the config structure
        // or inject arbitrary scripts into the `postrotate` block.
        if log_dir.contains('\n')
            || log_dir.contains('{')
            || log_dir.contains('}')
            || log_dir.contains(';')
        {
            return Err("SECURITY VIOLATION: log_dir contains illegal characters".into());
        }

        let config_path = format!("{}/kari-{}", self.logrotate_dir, domain_name);

        // 🛡️ 3. Platform-Agnostic Process Signaling
        // We use `systemctl reload nginx` instead of a hardcoded `/var/run/nginx.pid` path.
        // This lets systemd locate the correct PID natively, regardless of the Linux distribution.
        let logrotate_config = format!(
            r#"{log_dir}/*.log {{
    daily
    missingok
    rotate 14
    compress
    delaycompress
    notifempty
    create 640 root root
    sharedscripts
    postrotate
        systemctl reload nginx > /dev/null 2>/dev/null || true
    endscript
}}
"#,
            log_dir = log_dir
        );

        tokio_fs::write(&config_path, logrotate_config)
            .await
            .map_err(|e| format!("Failed to write logrotate config: {}", e))?;

        // 🛡️ 4. Native Kernel Syscalls (No `chmod` subprocess)
        // Logrotate daemon is highly strict. If the file is writable by group/world (e.g. 666),
        // it will refuse to execute it. We enforce 644 strictly via the kernel.
        let mut perms = tokio_fs::metadata(&config_path)
            .await
            .map_err(|e| format!("Failed to read logrotate config metadata: {}", e))?
            .permissions();

        perms.set_mode(0o644);
        tokio_fs::set_permissions(&config_path, perms)
            .await
            .map_err(|e| format!("Failed to secure logrotate config permissions: {}", e))?;

        Ok(())
    }
}
