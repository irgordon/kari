use crate::sys::traits::GitManager;
use async_trait::async_trait;
use tokio::process::Command;
use std::io::Write;
use tempfile::NamedTempFile;

pub struct SystemGitManager;

impl SystemGitManager {
    /// 🛡️ SLA Scrubber: Uses a more aggressive redaction strategy for git logs
    fn scrub_credentials(input: &str) -> String {
        // Redacts credentials in https://[TOKEN]@github.com or git@[TOKEN]:repo formats
        let re = regex::Regex::new(r"(://|git@)([^@]+)@").unwrap();
        re.replace_all(input, "$1[REDACTED]@").to_string()
    }
}

#[async_trait]
impl GitManager for SystemGitManager {
    async fn clone_repo(
        &self, 
        repo_url: &str, 
        branch: &str, 
        target_dir: &str,
        ssh_key: Option<&str> // 🛡️ Karı 2026: Transient SSH Support
    ) -> Result<(), String> {
        
        // 🛡️ 1. Zero-Trust Guard: Argument Injection Protection
        if repo_url.starts_with('-') || branch.starts_with('-') {
            return Err("SECURITY VIOLATION: Suspicious git arguments detected".into());
        }

        // 🛡️ 2. Transient SSH Identity Setup
        // We write the key to a memory-backed temp file that is purged on function exit.
        let mut _key_file = None;
        let mut git_ssh_cmd = "ssh -o StrictHostKeyChecking=accept-new -o IdentitiesOnly=yes".to_string();

        if let Some(key) = ssh_key {
            let mut temp = NamedTempFile::new().map_err(|e| e.to_string())?;
            temp.write_all(key.as_bytes()).map_err(|e| e.to_string())?;
            let path = temp.path().to_str().ok_or("Invalid path")?;
            git_ssh_cmd.push_str(&format!(" -i {}", path));
            _key_file = Some(temp); // Keep file alive until clone finishes
        }

        // 🛡️ 3. Execution with Recursive Hardening
        let output = Command::new("git")
            .arg("-c").arg("core.hooksPath=/dev/null") 
            .env("GIT_TERMINAL_PROMPT", "0")
            .env("GIT_SSH_COMMAND", git_ssh_cmd) // Inject the transient identity
            .arg("clone")
            .arg("--depth").arg("1")
            .arg("--branch").arg(branch)
            .arg("--recurse-submodules") // Support complex dependency trees
            .arg("--shallow-submodules") // Keep footprint low
            .arg("--") // End of options
            .arg(repo_url)
            .arg(target_dir)
            .output()
            .await
            .map_err(|e| format!("SLA Failure: Git spawn error: {}", e))?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            let sanitized = Self::scrub_credentials(&stderr.replace(repo_url, "[REPO_URL]"));
            return Err(format!("Git Sync Failed: {}", sanitized));
        }

        Ok(())
    }
}
