use crate::sys::secrets::ProviderCredential;
use crate::sys::traits::GitManager;
use async_trait::async_trait;
use std::io::{Seek, SeekFrom, Write};
use std::os::unix::fs::PermissionsExt;
use std::path::Path;
use std::sync::LazyLock;
use tempfile::NamedTempFile;
use tokio::process::Command;

pub struct SystemGitManager;

// 🛡️ SLA Performance: Compile the regex ONCE at boot time, not on every clone failure.
// `LazyLock` is the modern (Rust 1.80+) standard for safe static initialization.
static CREDENTIAL_SCRUBBER: LazyLock<regex::Regex> = LazyLock::new(|| {
    regex::Regex::new(r"(://|git@)([^@]+)@").expect("Failed to compile git credential regex")
});

impl SystemGitManager {
    /// 🛡️ SLA Scrubber: Uses a more aggressive redaction strategy for git logs
    fn scrub_credentials(input: &str) -> String {
        CREDENTIAL_SCRUBBER
            .replace_all(input, "$1[REDACTED]@")
            .to_string()
    }
}

#[async_trait]
impl GitManager for SystemGitManager {
    async fn clone_repo(
        &self,
        repo_url: &str,
        branch: &str,
        target_dir: &Path,                   // 🛡️ SLA: Strict Type
        ssh_key: Option<ProviderCredential>, // 🛡️ Zero-Trust: Enforce Memory Hygiene
    ) -> Result<(), String> {
        // 1. 🛡️ Zero-Trust Guard: Argument Injection Protection
        if repo_url.starts_with('-') || branch.starts_with('-') {
            return Err("SECURITY VIOLATION: Suspicious git arguments detected".into());
        }

        // 2. 🛡️ Transient SSH Identity Setup
        let mut key_file_guard = None;
        let mut git_ssh_cmd =
            "ssh -o StrictHostKeyChecking=accept-new -o IdentitiesOnly=yes".to_string();

        if let Some(cred) = ssh_key {
            let mut temp = NamedTempFile::new().map_err(|e| format!("Temp file error: {}", e))?;

            // 🛡️ Explicitly enforce 0600 permissions. SSH will reject the key if it's too open.
            let mut perms = std::fs::metadata(temp.path())
                .map_err(|e| e.to_string())?
                .permissions();
            perms.set_mode(0o600);
            std::fs::set_permissions(temp.path(), perms).map_err(|e| e.to_string())?;

            // Lexical confinement: Read the secret, write it to the temp file, and immediately drop it.
            cred.use_secret(|secret_str| temp.write_all(secret_str.as_bytes()))
                .map_err(|e| format!("Failed to write SSH key: {}", e))?;

            temp.as_file().sync_all().map_err(|e| e.to_string())?;

            let path = temp.path().to_str().ok_or("Invalid UTF-8 in temp path")?;

            // Wrap path in quotes to prevent shell injection via malicious temp directories
            git_ssh_cmd.push_str(&format!(" -i '{}'", path));

            key_file_guard = Some(temp);

            // Proactively scrub the RAM buffer now that it's on disk
            cred.destroy();
        }

        let target_dir_str = target_dir.to_str().ok_or("Invalid UTF-8 in target path")?;

        // 3. Execution with Recursive Hardening
        let output = Command::new("git")
            .arg("-c")
            .arg("core.hooksPath=/dev/null")
            .env("GIT_TERMINAL_PROMPT", "0")
            .env("GIT_SSH_COMMAND", git_ssh_cmd)
            .arg("clone")
            .arg("--depth")
            .arg("1")
            .arg("--branch")
            .arg(branch)
            .arg("--recurse-submodules")
            .arg("--shallow-submodules")
            .arg("--")
            .arg(repo_url)
            .arg(target_dir_str)
            .kill_on_drop(true) // 🛡️ SLA: Context propagation drops the process
            .output()
            .await
            .map_err(|e| format!("SLA Failure: Git spawn error: {}", e))?;

        // 4. 🛡️ Disk Residue Scrubbing
        // Regardless of git clone success or failure, we physically overwrite the SSH key on the SSD.
        if let Some(mut temp) = key_file_guard {
            // Seek back to the start of the file
            let _ = temp.seek(SeekFrom::Start(0));
            // Overwrite with 4KB of zeroes (covers max RSA key sizes)
            let _ = temp.write_all(&[0u8; 4096]);
            let _ = temp.as_file().sync_all();
            // File is cleanly dropped and unlinked at the end of this scope.
        }

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            let sanitized = Self::scrub_credentials(&stderr.replace(repo_url, "[REPO_URL]"));
            return Err(format!("Git Sync Failed: {}", sanitized));
        }

        Ok(())
    }
}
