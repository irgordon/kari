// agent/src/sys/scheduler.rs

use crate::sys::traits::{JobIntent, JobScheduler};
use async_trait::async_trait;
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
        let service_name = format!("kari-job-{}", intent.name);
        let service_path = format!("{}/{}.service", self.systemd_dir, service_name);
        let timer_path = format!("{}/{}.timer", self.systemd_dir, service_name);

        // 1. Generate the Execution Unit (.service)
        // Notice Type=oneshot: This tells systemd the process will exit when done,
        // preventing systemd from constantly trying to "restart" it like a web server.
        let service_content = format!(
            r#"[Unit]
Description=Kari Scheduled Job: {job_name}
After=network.target

[Service]
Type=oneshot
User={user}
Group={user}
ExecStart={command}

# --- 🛡️ Kari Ironclad Security Directives ---
NoNewPrivileges=true
ProtectSystem=full
PrivateTmp=true
ProtectHome=true
ProtectKernelTunables=true
ProtectControlGroups=true
"#,
            job_name = intent.name,
            user = intent.user,
            command = intent.command
        );

        // 2. Generate the Scheduling Unit (.timer)
        // Modern systemd (230+) natively accepts standard CRON expressions 
        // in the OnCalendar directive, making our SLA perfectly backwards-compatible.
        let timer_content = format!(
            r#"[Unit]
Description=Kari Timer for {job_name}

[Timer]
OnCalendar={schedule}
# If the server is down when the timer is supposed to fire, 
# trigger it immediately upon boot.
Persistent=true
AccuracySec=1s

[Install]
WantedBy=timers.target
"#,
            job_name = intent.name,
            schedule = intent.schedule
        );

        // 3. Write files safely to disk (using injected configuration paths)
        fs::write(&service_path, service_content)
            .await
            .map_err(|e| format!("Failed to write service file: {}", e))?;

        fs::write(&timer_path, timer_content)
            .await
            .map_err(|e| format!("Failed to write timer file: {}", e))?;

        // 4. Lock permissions to root
        for path in [&service_path, &timer_path] {
            let chmod_out = Command::new("chmod")
                .args(["644", path])
                .output()
                .await
                .map_err(|e| format!("Failed to execute chmod: {}", e))?;

            if !chmod_out.status.success() {
                return Err(format!("Failed to secure permissions for {}", path));
            }
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
            return Err(format!("Failed to activate timer {}", timer_name));
        }

        Ok(())
    }
}
