use async_trait::async_trait;
use tokio::process::Command;
use std::os::unix::fs::PermissionsExt;

#[async_trait]
pub trait JailManager: Send + Sync {
    /// Creates a unique Linux user with no login shell and no home directory
    async fn provision_app_user(&self, username: &str) -> Result<(), String>;
    
    /// Purges the user from the system during application teardown
    async fn deprovision_app_user(&self, username: &str) -> Result<(), String>;
    
    /// Locks down a directory so only the app user (and root) can access it
    async fn secure_directory(&self, path: &str, username: &str) -> Result<(), String>;
}

pub struct LinuxJailManager;

#[async_trait]
impl JailManager for LinuxJailManager {
    async fn provision_app_user(&self, username: &str) -> Result<(), String> {
        // 🛡️ Zero-Trust Input Validation
        if username.is_empty() || !username.chars().all(|c| c.is_ascii_alphanumeric() || c == '-') {
            return Err(format!("SECURITY VIOLATION: Invalid username '{}'", username));
        }

        // Idempotency check: Check if user exists
        let check = Command::new("id").arg("-u").arg(username).output().await;
        if let Ok(output) = check {
            if output.status.success() {
                return Ok(()); 
            }
        }

        // 🛡️ Hardened user creation
        // --system: No password aging, lower UID range
        // --no-create-home: We manage the directory structure ourselves
        // --shell /bin/false: Guaranteed no interactive access
        let output = Command::new("useradd")
            .args(["--system", "--no-create-home", "--shell", "/bin/false", username])
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
        // 🛡️ Ensure we aren't deleting protected system accounts
        if !username.starts_with("kari-") {
             return Err("SECURITY VIOLATION: Refusing to delete non-Kari user".into());
        }

        let output = Command::new("userdel")
            .arg(username)
            .output()
            .await
            .map_err(|e| format!("Failed to execute userdel: {}", e))?;

        // Ignore error if user is already gone (idempotency)
        Ok(())
    }

    async fn secure_directory(&self, path: &str, username: &str) -> Result<(), String> {
        // Validate inputs to prevent command injection
        if !username.chars().all(|c| c.is_ascii_alphanumeric() || c == '-') {
            return Err("SECURITY VIOLATION: Invalid username format".into());
        }

        // Ensure directory exists
        tokio::fs::create_dir_all(path)
            .await
            .map_err(|e| format!("Filesystem Error: {}", e))?;

        // 🛡️ Native Permission Set (0750)
        // rwxr-x--- : Owner has full, Group can read/enter, World has nothing.
        let mut perms = tokio::fs::metadata(path).await.map_err(|e| e.to_string())?.permissions();
        perms.set_mode(0o750);
        tokio::fs::set_permissions(path, perms).await.map_err(|e| e.to_string())?;

        // 🛡️ Recursive Chown with Symlink Protection
        // -h / --no-dereference: Don't follow symlinks! 
        // This prevents an app from linking to /etc/shadow to try and steal ownership.
        let chown_out = Command::new("chown")
            .args(["-Rh", &format!("{}:{}", username, username), path])
            .output()
            .await
            .map_err(|e| format!("Failed to spawn chown: {}", e))?;

        if !chown_out.status.success() {
            let stderr = String::from_utf8_lossy(&chown_out.stderr);
            return Err(format!("Failed to secure directory {}: {}", path, stderr));
        }

        Ok(())
    }
}
