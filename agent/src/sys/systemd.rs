// agent/src/sys/systemd.rs

use async_trait::async_trait;
use std::collections::HashMap;
use tokio::fs;
use tokio::process::Command;

pub struct ServiceConfig {
    pub service_name: String,
    pub username: String,
    pub working_directory: String,
    pub start_command: String,
    pub env_vars: HashMap<String, String>,
}

#[async_trait]
pub trait ServiceManager: Send + Sync {
    async fn write_unit_file(&self, config: &ServiceConfig) -> Result<(), String>;
    async fn reload_daemon(&self) -> Result<(), String>;
    async fn enable_and_start(&self, service_name: &str) -> Result<(), String>;
}

pub struct LinuxSystemdManager {
    systemd_dir: String, // Injected path
}

impl LinuxSystemdManager {
    pub fn new(systemd_dir: String) -> Self {
        Self { systemd_dir }
    }
}

#[async_trait]
impl ServiceManager for LinuxSystemdManager {
    async fn write_unit_file(&self, config: &ServiceConfig) -> Result<(), String> {
        // INJECTED: Dynamically construct the path
        let path = format!("{}/{}.service", self.systemd_dir, config.service_name);
        
        let mut env_strings = String::new();
        for (k, v) in &config.env_vars {
            env_strings.push_str(&format!("Environment=\"{}={}\"\n", k, v));
        }

        let unit_content = format!(
            r#"[Unit]
Description=Kari Managed App: {service_name}
After=network.target

[Service]
Type=simple
User={username}
Group={username}
WorkingDirectory={workdir}
ExecStart={exec_start}
{env_block}
Restart=always
RestartSec=3

# --- ⚖️ CGroup Resource Limits (Prevent Noisy Neighbors) ---
CPUAccounting=true
CPUQuota=100%
MemoryAccounting=true
MemoryMax=512M
TasksMax=512

# --- 🛡️ Kari Ironclad Security Directives ---
NoNewPrivileges=true
ProtectSystem=full
PrivateTmp=true
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true

[Install]
WantedBy=multi-user.target
"#,
            service_name = config.service_name,
            username = config.username,
            workdir = config.working_directory,
            exec_start = config.start_command,
            env_block = env_strings
        );

        fs::write(&path, unit_content)
            .await
            .map_err(|e| format!("Failed to write systemd unit: {}", e))?;

        Command::new("chmod").args(["644", &path]).output().await.map_err(|e| e.to_string())?;

        Ok(())
    }

    async fn reload_daemon(&self) -> Result<(), String> {
        let output = Command::new("systemctl").arg("daemon-reload").output().await.map_err(|e| e.to_string())?;
        if !output.status.success() {
            return Err("Failed to reload systemd daemon".into());
        }
        Ok(())
    }

    async fn enable_and_start(&self, service_name: &str) -> Result<(), String> {
        Command::new("systemctl").args(["enable", "--now", service_name]).output().await.map_err(|e| e.to_string())?;
        Ok(())
    }
}
