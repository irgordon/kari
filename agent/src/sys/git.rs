// agent/src/sys/git.rs

use crate::sys::traits::GitManager;
use async_trait::async_trait;
use tokio::process::Command;

pub struct SystemGitManager;

impl SystemGitManager {
    /// 🛡️ SLA Enforcement: Scrubber to prevent credentials from leaking into Audit Logs
    fn scrub_credentials(url: &str) -> String {
        // Looks for `://[credentials]@` and redacts them
        if let Some(scheme_idx) = url.find("://") {
            let after_scheme = &url[scheme_idx + 3..];
            if let Some(at_idx) = after_scheme.find('@') {
                return format!("{}://[REDACTED]...WARNING_AUTH_HIDDEN@{}", &url[..scheme_idx], &after_scheme[at_idx + 1..]);
            }
        }
        url.to_string()
    }
}

#[async_trait]
impl GitManager for SystemGitManager {
    async fn clone_repo(&self, repo_url: &str, branch: &str, target_dir: &str) -> Result<(), String> {
        
        // 🛡️ 1. Zero-Trust Input Validation (Anti-Argument Injection)
        // Ensure URLs and branches don't start with hyphens to prevent flag injection.
        if repo_url.starts_with('-') || branch.starts_with('-') {
            return Err("SECURITY VIOLATION: Git arguments cannot start with a hyphen".into());
        }

        // 🛡️ 2. Platform-Agnostic Sandbox Execution
        // Run: git clone --depth 1 --branch <branch> -- <url> <target>
        let output = Command::new("git")
            // Disable local hooks to prevent RCE from attacker-controlled repos
            .arg("-c").arg("core.hooksPath=/dev/null") 
            // Disable terminal prompts (e.g., asking for a password) hanging the async task
            .env("GIT_TERMINAL_PROMPT", "0")
            .arg("clone")
            .arg("--depth").arg("1") // Shallow clone for SLA speed
            .arg("--branch").arg(branch)
            .arg("--") // 🛡️ End of Options Delimiter: Forces git to treat remaining args as positional
            .arg(repo_url)
            .arg(target_dir)
            .output()
            .await
            .map_err(|e| format!("Failed to spawn git process: {}", e))?;

        if !output.status.success() {
            let raw_stderr = String::from_utf8_lossy(&output.stderr);
            
            // 🛡️ 3. Credential Scrubbing
            // Replace the plaintext token in the error message before it goes back to the Go Brain
            let safe_url = Self::scrub_credentials(repo_url);
            let sanitized_stderr = raw_stderr.replace(repo_url, &safe_url);
            
            return Err(format!("Git clone failed: {}", sanitized_stderr));
        }

        Ok(())
    }
}
