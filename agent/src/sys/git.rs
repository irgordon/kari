// agent/src/sys/git.rs

use crate::sys::traits::GitManager;
use async_trait::async_trait;
use tokio::process::Command;

pub struct SystemGitManager;

#[async_trait]
impl GitManager for SystemGitManager {
    async fn clone_repo(&self, repo_url: &str, branch: &str, target_dir: &str) -> Result<(), String> {
        // Run: git clone --depth 1 --branch <branch> <url> <target>
        let output = Command::new("git")
            .arg("clone")
            .arg("--depth").arg("1") // Shallow clone for speed
            .arg("--branch").arg(branch)
            .arg(repo_url)
            .arg(target_dir)
            .output()
            .await
            .map_err(|e| format!("Failed to spawn git process: {}", e))?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            return Err(format!("Git clone failed: {}", stderr));
        }

        Ok(())
    }
}
