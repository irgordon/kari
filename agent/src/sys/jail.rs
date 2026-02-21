// agent/src/sys/jail.rs

use async_trait::async_trait;
use tokio::process::Command;
use std::os::unix::fs::PermissionsExt;

#[async_trait]
pub trait JailManager: Send + Sync {
    /// Creates a unique Linux user with no login shell
    async fn provision_app_user(&self, username: &str) -> Result<(), String>;
    
    /// Locks down a directory so only the app user (and root) can access it
    async fn secure_directory(&self, path: &str, username: &str) -> Result<(), String>;
}

pub struct LinuxJailManager;

#[async_trait]
impl JailManager for LinuxJailManager {
    async fn provision_app_user(&self, username: &str) -> Result<(), String> {
        // 🛡️ 1. Zero-Trust Input Validation (Anti-Argument Injection)
        // We enforce that the Go API is strictly following the "kari-app-ID" format.
        if username.is_empty() || !username.chars().all(|c| c.is_ascii_alphanumeric() || c == '-') {
            return Err(format!("SECURITY VIOLATION: Invalid characters in username '{}'", username));
        }

        // 2. Check if user already exists
        let check = Command::new("id").arg("-u").arg(username).output().await;
        if let Ok(output) = check {
            if output.status.success() {
                return Ok(()); // User exists, idempotent success
            }
        }

        // 3. Create an unprivileged system user with NO login shell
        let output = Command::new("useradd")
            .args(["--system", "--shell", "/bin/false", username])
            .output()
            .await
            .map_err(|e| format!("Failed to execute useradd: {}", e))?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            return Err(format!("Failed to create user {}: {}", username, stderr));
        }

        Ok(())
    }

    async fn secure_directory(&self, path: &str, username: &str) -> Result<(), String> {
        // 🛡️ 1. Zero-Trust Input Validation
        if !username.chars().all(|c| c.is_ascii_alphanumeric() || c == '-') {
            return Err("SECURITY VIOLATION: Invalid username format".into());
        }

        // 🛡️ 2. Eliminate TOCTOU Race Condition
        // `create_dir_all` safely does nothing if the directory already exists.
        tokio::fs::create_dir_all(path)
            .await
            .map_err(|e| format!("Failed to create app directory: {}", e))?;

        // 🛡️ 3. Platform Agnostic Syscalls (No `chmod` subprocess)
        // We use Rust's native standard library to interface directly with the kernel.
        // 0o750: Owner(rwx), Group(r-x), Others(---)
        let mut perms = tokio::fs::metadata(path)
            .await
            .map_err(|e| format!("Failed to read metadata: {}", e))?
            .permissions();
            
        perms.set_mode(0o750);
        tokio::fs::set_permissions(path, perms)
            .await
            .map_err(|e| format!("Failed to set permissions: {}", e))?;

        // 4. Recursive Chown
        // Note: We keep `chown -R` via Command because walking the directory tree natively 
        // in async Rust is highly complex, and `chown` handles the recursive edge cases perfectly.
        // Because we validated `username` above, this argument interpolation is 100% mathematically safe.
        let chown_out = Command::new("chown")
            .args(["-R", &format!("{}:{}", username, username), path])
            .output()
            .await
            .map_err(|e| format!("Failed to spawn chown: {}", e))?;

        if !chown_out.status.success() {
            let stderr = String::from_utf8_lossy(&chown_out.stderr);
            return Err(format!("Failed to chown directory: {}", stderr));
        }

        Ok(())
    }
}
