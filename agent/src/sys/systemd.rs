use async_trait::async_trait;
use std::collections::HashMap;
use std::os::unix::fs::PermissionsExt;
use std::path::PathBuf;
use tokio::fs;
use tokio::process::Command;

// 🛡️ SLA: Domain Intent mapped to Rust Execution
pub struct ServiceConfig {
    pub service_name: String,
    pub username: String,
    pub working_directory: PathBuf, // 🛡️ SLA: Strict Type
    pub start_command: String,
    pub env_vars: HashMap<String, String>,
    pub memory_limit_mb: i32,
    pub cpu_limit_percent: i32,
}

#[async_trait]
pub trait ServiceManager: Send + Sync {
    async fn write_unit_file(&self, config: &ServiceConfig) -> Result<(), String>;
    async fn remove_unit_file(&self, service_name: &str) -> Result<(), String>;
    async fn reload_daemon(&self) -> Result<(), String>;
    async fn enable_and_start(&self, service_name: &str) -> Result<(), String>;
    async fn start(&self, service_name: &str) -> Result<(), String>;
    async fn stop(&self, service_name: &str) -> Result<(), String>;
    async fn restart(&self, service_name: &str) -> Result<(), String>;
}

pub struct LinuxSystemdManager {
    systemd_dir: PathBuf,
}

impl LinuxSystemdManager {
    pub fn new(systemd_dir: PathBuf) -> Self {
        Self { systemd_dir }
    }

    /// 🛡️ Zero-Trust: Safely joins paths to prevent unit file hijacking
    fn get_unit_path(&self, service_name: &str) -> Result<PathBuf, String> {
        // Prevent path traversal attacks (e.g. "../../../etc/shadow")
        if service_name.contains("..") || service_name.contains('/') {
            return Err("SECURITY VIOLATION: Path traversal in service name".into());
        }
        // Force the .service extension so they can't overwrite arbitrary system files
        Ok(self.systemd_dir.join(format!("{}.service", service_name)))
    }

    async fn execute_systemctl(&self, args: &[&str]) -> Result<(), String> {
        let output = Command::new("systemctl")
            .args(args)
            .output()
            .await
            .map_err(|e| format!("SLA Failure: systemctl execution error: {}", e))?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            return Err(format!("systemctl {} failed: {}", args[0], stderr));
        }
        Ok(())
    }
}

#[async_trait]
impl ServiceManager for LinuxSystemdManager {
    async fn write_unit_file(&self, config: &ServiceConfig) -> Result<(), String> {
        let path = self.get_unit_path(&config.service_name)?;
        
        // 1. 🛡️ Secure Environment Block Generation (Strict POSIX Validation)
        let mut env_block = String::new();
        for (k, v) in &config.env_vars {
            // Keys MUST be strictly alphanumeric and underscores.
            // This prevents systemd directive injection via malicious keys.
            if !k.chars().all(|c| c.is_ascii_alphanumeric() || c == '_') {
                tracing::warn!("Dropping invalid environment variable key: {}", k);
                continue;
            }
            
            // Values: Escape double quotes and backslashes for safe systemd parsing
            let safe_v = v.replace('\\', "\\\\").replace('"', "\\\"");
            env_block.push_str(&format!("Environment=\"{}={}\"\n", k, safe_v));
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
RestartSec=5

# --- ⚖️ Dynamic Resource Jailing ---
CPUAccounting=true
CPUQuota={cpu_limit}%
MemoryAccounting=true
MemoryMax={mem_limit}M
TasksMax=512

# --- 🛡️ Hardened Sandbox (2026 Grade) ---
NoNewPrivileges=true
ProtectSystem=strict
PrivateTmp=true
ProtectHome=true
PrivateDevices=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
CapabilityBoundingSet=
RestrictRealtime=true
RestrictSUIDSGID=true
ReadWritePaths={workdir}

[Install]
WantedBy=multi-user.target
"#,
            service_name = config.service_name,
            username = config.username,
            workdir = config.working_directory.to_string_lossy(),
            exec_start = config.start_command, // Trusted via upstream validation
            env_block = env_block,
            cpu_limit = config.cpu_limit_percent,
            mem_limit = config.memory_limit_mb
        );

        // Write the file to disk
        fs::write(&path, unit_content).await.map_err(|e| e.to_string())?;
        
        // 2. 🛡️ Ensure standard 644 permissions (rw-r--r--)
        let mut perms = fs::metadata(&path).await.map_err(|e| e.to_string())?.permissions();
        perms.set_mode(0o644);
        fs::set_permissions(&path, perms).await.map_err(|e| e.to_string())?;

        Ok(())
    }

    async fn remove_unit_file(&self, service_name: &str) -> Result<(), String> {
        let path = self.get_unit_path(service_name)?;
        if path.exists() {
            fs::remove_file(&path).await.map_err(|e| format!("Cleanup failed: {}", e))?;
        }
        Ok(())
    }

    async fn reload_daemon(&self) -> Result<(), String> {
        self.execute_systemctl(&["daemon-reload"]).await
    }

    async fn enable_and_start(&self, service_name: &str) -> Result<(), String> {
        self.execute_systemctl(&["enable", "--now", service_name]).await
    }

    async fn start(&self, service_name: &str) -> Result<(), String> {
        self.execute_systemctl(&["start", service_name]).await
    }

    async fn stop(&self, service_name: &str) -> Result<(), String> {
        self.execute_systemctl(&["stop", service_name]).await
    }

    async fn restart(&self, service_name: &str) -> Result<(), String> {
        self.execute_systemctl(&["restart", service_name]).await
    }
}
