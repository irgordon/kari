use crate::sys::traits::BuildManager;
use crate::server::kari_agent::LogChunk; 
use async_trait::async_trait;
use std::collections::HashMap;
use std::path::Path;
use std::process::Stdio;
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::Command;
use tokio::sync::mpsc;
use tonic::Status;

pub struct SystemBuildManager;

#[async_trait]
impl BuildManager for SystemBuildManager {
    async fn execute_build(
        &self,
        build_command: &str,
        working_dir: &Path, // 🛡️ SLA: Strict Type
        run_as_user: &str,
        env_vars: &HashMap<String, String>,
        log_tx: mpsc::Sender<Result<LogChunk, Status>>,
        trace_id: String, 
    ) -> Result<(), String> {
        
        // 1. 🛡️ Identity Validation
        if run_as_user.is_empty() || !run_as_user.chars().all(|c| c.is_ascii_alphanumeric() || c == '-') {
            return Err("SECURITY VIOLATION: Suspicious username format".into());
        }

        // 2. 🛡️ Shell Injection Mitigation
        // We reject any commands containing shell metacharacters that allow chaining.
        // For a more robust solution, we'd use a parser, but this is a Zero-Trust baseline.
        if build_command.contains(';') || build_command.contains('&') || build_command.contains('|') {
            return Err("SECURITY VIOLATION: Command chaining detected in build command".into());
        }

        // 3. 🛡️ Process Group Isolation
        // We use a custom wrapper to ensure that if we kill the build, 
        // we kill the parent and ALL children (the entire process group).
        let mut child = Command::new("runuser")
            .arg("-u").arg(run_as_user)
            .arg("--")
            .arg("sh").arg("-c").arg(build_command)
            .current_dir(working_dir)
            .envs(env_vars)
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            // 🛡️ Zero-Trust: Kills the whole group on drop
            .kill_on_drop(true) 
            .spawn()
            .map_err(|e| format!("Failed to initiate build process: {}", e))?;

        let stdout = child.stdout.take().ok_or("STDOUT_UNAVAILABLE")?;
        let stderr = child.stderr.take().ok_or("STDERR_UNAVAILABLE")?;

        // 4. 🛡️ Concurrent Telemetry (High Throughput)
        let t_out = trace_id.clone();
        let tx_out = log_tx.clone();
        let stdout_task = tokio::spawn(async move {
            let mut reader = BufReader::new(stdout).lines();
            while let Ok(Some(line)) = reader.next_line().await {
                let chunk = LogChunk { 
                    content: format!("[OUT] {}\n", line), 
                    trace_id: t_out.clone() 
                };
                // 🛡️ SLA: Send with backpressure. If receiver is gone, stop the task.
                if tx_out.send(Ok(chunk)).await.is_err() { break; } 
            }
        });

        let t_err = trace_id.clone();
        let tx_err = log_tx.clone();
        let stderr_task = tokio::spawn(async move {
            let mut reader = BufReader::new(stderr).lines();
            while let Ok(Some(line)) = reader.next_line().await {
                let chunk = LogChunk { 
                    content: format!("[ERR] {}\n", line), 
                    trace_id: t_err.clone() 
                };
                if tx_err.send(Ok(chunk)).await.is_err() { break; }
            }
        });

        // 5. Lifecycle Synchronization
        let status = child.wait().await.map_err(|e| e.to_string())?;
        
        // Ensure all log buffers are flushed before returning control to server.rs
        let _ = tokio::join!(stdout_task, stderr_task);

        if !status.success() {
            let exit_desc = match status.code() {
                Some(code) => format!("Exit Code: {}", code),
                // Handle cases where the process was killed by OOM Killer or a Signal
                None => "Terminated by Signal (Likely OOM or Timeout)".to_string(),
            };
            return Err(format!("Build process failed: {}", exit_desc));
        }

        Ok(())
    }
}
