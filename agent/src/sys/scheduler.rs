// agent/src/sys/scheduler.rs

use crate::sys::traits::{JobIntent, JobScheduler};
use async_trait::async_trait;
use std::os::unix::fs::PermissionsExt;
use tokio::fs;
use tokio::process::Command;

// ==============================================================================
// 1. Concrete Implementation (Systemd)
// ==============================================================================

pub struct SystemdTimerManager {
    systemd_dir: String, // Injected via AgentConfig, e.g., "/etc/systemd/system"
}

impl SystemdTimerManager {
    pub fn new(systemd_dir: String) -> Self {
        Self { systemd_dir }
    }
}

#[async_trait]
impl JobScheduler for SystemdTimerManager {
    async fn schedule_job(&self, intent: &JobIntent) -> Result<(), String> {
        // 🛡️ 1. Zero-Trust Path Traversal Shield
        if intent.name.is_empty()
            || !intent
                .name
                .chars()
                .all(|c| c.is_ascii_alphanumeric() || c == '-')
        {
            return Err("SECURITY VIOLATION: Invalid job name format".into());
        }

        // 🛡️ 2. Directive Injection Prevention
        if intent.schedule.contains('\n') || intent.schedule.contains('=') {
            return Err("SECURITY VIOLATION: Invalid characters in schedule".into());
        }

        let service_name = format!("kari-job-{}", intent.name);
        let service_path = format!("{}/{}.service", self.systemd_dir, service_name);
        let timer_path = format!("{}/{}.timer", self.systemd_dir, service_name);

        // 🛡️ 3. SLA Trait Compliance (Anti-Injection Construction)
        // We iterate through the discrete arguments and safely quote them for systemd parsing,
        // completely avoiding raw string execution.
        let mut exec_start = intent.binary.clone();
        for arg in &intent.args {
            let safe_arg = arg.replace('"', "\\\"");
            exec_start.push_str(&format!(" \"{}\"", safe_arg));
        }

        // Generate the Execution Unit (.service)
        let service_content = format!(
            r#"[Unit]
Description=Kari Scheduled Job: {job_name}
After=network.target

[Service]
Type=oneshot
User={user}
Group={user}
ExecStart={exec_start}

# --- 🛡️ Kari Ironclad Security Directives ---
NoNewPrivileges=true
ProtectSystem=full
PrivateTmp=true
ProtectHome=true
ProtectKernelTunables=true
ProtectControlGroups=true
"#,
            job_name = intent.name,
            user = intent.run_as_user, // Satisfies the new trait contract
            exec_start = exec_start
        );

        // Generate the Scheduling Unit (.timer)
        let timer_content = format!(
            r#"[Unit]
Description=Kari Timer for {job_name}

[Timer]
OnCalendar={schedule}
Persistent=true
AccuracySec=1s

[Install]
WantedBy=timers.target
"#,
            job_name = intent.name,
            schedule = intent.schedule
        );

        // Write files safely to disk
        fs::write(&service_path, service_content)
            .await
            .map_err(|e| format!("Failed to write service file: {}", e))?;

        fs::write(&timer_path, timer_content)
            .await
            .map_err(|e| format!("Failed to write timer file: {}", e))?;

        // 🛡️ 4. Native Kernel Syscalls (No `chmod` subprocess)
        for path in [&service_path, &timer_path] {
            let mut perms = fs::metadata(path)
                .await
                .map_err(|e| format!("Failed to read metadata for {}: {}", path, e))?
                .permissions();

            perms.set_mode(0o644);
            fs::set_permissions(path, perms)
                .await
                .map_err(|e| format!("Failed to secure permissions for {}: {}", path, e))?;
        }

        // 5. Reload daemon to recognize the new files
        let reload_out = Command::new("systemctl")
            .arg("daemon-reload")
            .output()
            .await
            .map_err(|e| format!("Failed to execute daemon-reload: {}", e))?;

        if !reload_out.status.success() {
            return Err("systemctl daemon-reload failed".into());
        }

        // 6. Enable and Start the Timer (Not the service!)
        let timer_name = format!("{}.timer", service_name);
        let enable_out = Command::new("systemctl")
            .args(["enable", "--now", &timer_name])
            .output()
            .await
            .map_err(|e| format!("Failed to enable timer: {}", e))?;

        if !enable_out.status.success() {
            let stderr = String::from_utf8_lossy(&enable_out.stderr);
            return Err(format!(
                "Failed to activate timer {}: {}",
                timer_name, stderr
            ));
        }

        Ok(())
    }
}
