// agent/src/sys/build.rs

use crate::sys::traits::BuildManager;
use async_trait::async_trait;
use std::collections::HashMap;
use std::process::Stdio;
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::Command;
use tokio::sync::mpsc;

pub struct SystemBuildManager;

#[async_trait]
impl BuildManager for SystemBuildManager {
    async fn execute_build(
        &self,
        build_command: &str,
        working_dir: &str,
        run_as_user: &str,
        env_vars: &HashMap<String, String>,
        log_sender: mpsc::Sender<String>,
    ) -> Result<(), String> {
        
        // Use `runuser` to drop privileges. 
        // We pass the build command to `bash -c` but ONLY under the context of the unprivileged user.
        let mut child = Command::new("runuser")
            .arg("-u").arg(run_as_user)
            .arg("--")
            .arg("bash").arg("-c").arg(build_command)
            .current_dir(working_dir)
            .envs(env_vars) // Inject the app's .env variables safely
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
            .map_err(|e| format!("Failed to spawn build process: {}", e))?;

        let stdout = child.stdout.take().expect("Failed to open stdout");
        let stderr = child.stderr.take().expect("Failed to open stderr");

        // Stream stdout to the channel
        let tx_out = log_sender.clone();
        tokio::spawn(async move {
            let mut reader = BufReader::new(stdout).lines();
            while let Ok(Some(line)) = reader.next_line().await {
                let _ = tx_out.send(format!("[STDOUT] {}\n", line)).await;
            }
        });

        // Stream stderr to the channel
        let tx_err = log_sender.clone();
        tokio::spawn(async move {
            let mut reader = BufReader::new(stderr).lines();
            while let Ok(Some(line)) = reader.next_line().await {
                let _ = tx_err.send(format!("[STDERR] {}\n", line)).await;
            }
        });

        let status = child.wait().await.map_err(|e| e.to_string())?;

        if !status.success() {
            return Err(format!("Build process exited with code: {}", status.code().unwrap_or(-1)));
        }

        Ok(())
    }
}
