use async_trait::async_trait;
use tokio::process::Command;
use std::path::Path;

#[async_trait]
pub trait JailManager: Send + Sync {
    /// 🛡️ SLA: The UID is dictated by the Brain's intent, not the OS's whims.
    async fn provision_app_user(&self, username: &str, uid: u32) -> Result<(), String>;
    
    /// Kills all user processes and purges the user from the system
    async fn deprovision_app_user(&self, username: &str) -> Result<(), String>;
    
    /// Locks down a directory safely, avoiding TOCTOU symlink races
    async fn secure_directory(&self, path: &Path, username: &str) -> Result<(), String>;
}

pub struct LinuxJailManager;

#[async_trait]
impl JailManager for LinuxJailManager {
    async fn provision_app_user(&self, username: &str, uid: u32) -> Result<(), String> {
        // 1. 🛡️ Zero-Trust Input Validation
        if username.is_empty() || !username.chars().all(|c| c.is_ascii_alphanumeric() || c == '-') {
            return Err(format!("SECURITY VIOLATION: Invalid username '{}'", username));
        }

        // Idempotency check: Does the user already exist?
        let check = Command::new("id").arg("-u").arg(username).output().await;
        if let Ok(output) = check {
            if output.status.success() {
                return Ok(()); 
            }
        }

        // 2. 🛡️ Deterministic Jailing
        // We force the specific UID passed from the Go API using `-u`.
        let output = Command::new("useradd")
            .args([
                "--system", 
                "--no-create-home", 
                "--shell", "/bin/false", 
                "-u", &uid.to_string(), 
                username
            ])
            .output()
            .await
            .map_err(|e| format!("SLA Failure: useradd spawn error: {}", e))?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            return Err(format!("Failed to provision user {}: {}", username, stderr));
        }

        Ok(())
    }

    async fn deprovision_app_user(&self, username: &str) -> Result<(), String> {
        if !username.starts_with("kari-") {
             return Err("SECURITY VIOLATION: Refusing to delete non-Kari user".into());
        }

        // 1. 🛡️ Hygiene: forcefully kill all lingering processes owned by this user
        // so `userdel` doesn't hang or fail.
        let _ = Command::new("killall")
            .args(["-u", username])
            .output()
            .await;

        // 2. Deterministic deletion
        let output = Command::new("userdel")
            .arg(username)
            .output()
            .await
            .map_err(|e| format!("Failed to execute userdel: {}", e))?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            // userdel returns exit code 6 if the user doesn't exist. We treat that as success.
            if output.status.code() != Some(6) {
                return Err(format!("Failed to deprovision user {}: {}", username, stderr));
            }
        }

        Ok(())
    }

    async fn secure_directory(&self, path: &Path, username: &str) -> Result<(), String> {
        if !username.chars().all(|c| c.is_ascii_alphanumeric() || c == '-') {
            return Err("SECURITY VIOLATION: Invalid username format".into());
        }

        tokio::fs::create_dir_all(path)
            .await
            .map_err(|e| format!("Filesystem Error: {}", e))?;

        // 🛡️ TOCTOU Mitigation & Recursive Symlink Safe-Chown
        // Rather than relying on non-atomic Rust fs calls, we delegate to the native 
        // Linux binaries which are battle-tested against symlink races when using specific flags.
        // `-P` prevents traversing symlinks that are encountered.
        let path_str = path.to_str().ok_or("Path contains invalid UTF-8")?;

        let chown_out = Command::new("chown")
            .args(["-RP", &format!("{}:{}", username, username), path_str])
            .output()
            .await
            .map_err(|e| format!("Failed to spawn chown: {}", e))?;

        if !chown_out.status.success() {
            return Err(format!("Failed to secure directory ownership: {}", String::from_utf8_lossy(&chown_out.stderr)));
        }

        // Apply strict 0750 permissions recursively
        let chmod_out = Command::new("chmod")
            .args(["-R", "0750", path_str])
            .output()
            .await
            .map_err(|e| format!("Failed to spawn chmod: {}", e))?;

        if !chmod_out.status.success() {
            return Err(format!("Failed to secure directory permissions: {}", String::from_utf8_lossy(&chmod_out.stderr)));
        }

        Ok(())
    }
}
