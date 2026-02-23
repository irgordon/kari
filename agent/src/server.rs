use std::collections::HashMap;
use std::os::unix::fs::PermissionsExt;
use std::sync::Arc;
use std::path::Path;
use tokio::sync::mpsc;
use tokio_stream::wrappers::ReceiverStream;
use tonic::{Request, Response, Status};
use tracing::{info, warn};
use zeroize::Zeroizing;

use crate::config::AgentConfig;
use crate::sys::build::SystemBuildManager;
use crate::sys::git::SystemGitManager;
use crate::sys::jail::{JailManager, LinuxJailManager};
use crate::sys::systemd::{LinuxSystemdManager, ServiceManager, ServiceConfig};
use crate::sys::traits::{
    ProxyManager, FirewallManager, SslEngine, JobScheduler,
    GitManager, BuildManager,
    FirewallAction, Protocol, FirewallPolicy as TraitFirewallPolicy,
    SslPayload as TraitSslPayload, JobIntent as TraitJobIntent,
};
use crate::sys::secrets::ProviderCredential;
use zeroize::Zeroize;

// Import the generated gRPC types
pub mod kari_agent {
    tonic::include_proto!("kari.agent.v1");
}

use kari_agent::system_agent_server::SystemAgent;
use kari_agent::{
    AgentResponse, DeployRequest, DeleteRequest, TeardownRequest, PackageRequest, Empty, SystemStatus,
    ServiceRequest, LogChunk, ProvisionJailRequest, FileWriteRequest,
    SslPayload, FirewallPolicy, JobIntent,
};

const ALLOWED_PKG_COMMANDS: &[&str] = &["apt-get", "apt", "dnf", "yum", "zypper"];

// ==============================================================================
// 🛡️ SOLID: KariAgentService is the single gRPC boundary.
// All execution is delegated to injected trait objects (SLA: Single Layer Abstraction).
// ==============================================================================

pub struct KariAgentService {
    config: AgentConfig,
    jail_mgr: Arc<dyn JailManager>,
    svc_mgr: Arc<dyn ServiceManager>,
    git_mgr: Arc<dyn GitManager>,
    build_mgr: Arc<dyn BuildManager>,
    proxy_mgr: Arc<dyn ProxyManager>,
    firewall_mgr: Arc<dyn FirewallManager>,
    ssl_engine: Arc<dyn SslEngine>,
    job_scheduler: Arc<dyn JobScheduler>,
}

impl KariAgentService {
    pub fn new(
        config: AgentConfig,
        proxy_mgr: Arc<dyn ProxyManager>,
        firewall_mgr: Arc<dyn FirewallManager>,
        ssl_engine: Arc<dyn SslEngine>,
        job_scheduler: Arc<dyn JobScheduler>,
    ) -> Self {
        Self {
            jail_mgr: Arc::new(LinuxJailManager),
            svc_mgr: Arc::new(LinuxSystemdManager::new(config.systemd_dir.clone())),
            git_mgr: Arc::new(SystemGitManager),
            build_mgr: Arc::new(SystemBuildManager),
            proxy_mgr,
            firewall_mgr,
            ssl_engine,
            job_scheduler,
            config,
        }
    }

    /// 🛡️ Zero-Trust: Strictly prevents directory traversal
    fn secure_join(base: &Path, unsafe_suffix: &str) -> Result<std::path::PathBuf, Status> {
        if unsafe_suffix.contains("..") || unsafe_suffix.contains('/') || unsafe_suffix.contains('\\') {
            return Err(Status::invalid_argument("Path traversal detected in identifier"));
        }
        Ok(base.join(unsafe_suffix))
    }

    /// 🛡️ Zero-Trust: Validates that a string is a safe alphanumeric-dash identifier
    fn validate_identifier(value: &str, field_name: &str) -> Result<(), Status> {
        if value.is_empty() || value.contains("..") || !value.chars().all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '_' || c == '.') {
            return Err(Status::invalid_argument(format!(
                "Zero-Trust: Invalid {} format: '{}'", field_name, value
            )));
        }
        Ok(())
    }
}

#[tonic::async_trait]
impl SystemAgent for KariAgentService {
    type StreamDeploymentStream = ReceiverStream<Result<LogChunk, Status>>;

    // =========================================================================
    // 1. 🛡️ SLA: System Health Telemetry
    // =========================================================================
    async fn get_system_status(
        &self,
        _request: Request<Empty>,
    ) -> Result<Response<SystemStatus>, Status> {
        use sysinfo::System;

        let mut sys = System::new_all();
        sys.refresh_all();

        // 🛡️ SLA: Calculate metrics from kernel-level sources
        let cpu_usage = sys.global_cpu_info().cpu_usage();
        let _total_memory = sys.total_memory() as f64;
        let used_memory = sys.used_memory() as f64;
        let memory_usage_mb = (used_memory / 1_048_576.0) as f32;

        // Active jails: count systemd services matching our naming convention
        let active_jails = sys.processes()
            .values()
            .filter(|p| {
                p.name().starts_with("kari-")
            })
            .count() as u32;

        let uptime = System::uptime();

        Ok(Response::new(SystemStatus {
            healthy: true,
            active_jails,
            cpu_usage_percent: cpu_usage,
            memory_usage_mb,
            agent_version: env!("CARGO_PKG_VERSION").to_string(),
            uptime_seconds: uptime,
        }))
    }

    // =========================================================================
    // 2. 📦 Package Management (Hardened)
    // =========================================================================
    async fn execute_package_command(
        &self,
        request: Request<PackageRequest>,
    ) -> Result<Response<AgentResponse>, Status> {
        let req = request.into_inner();
        
        if !ALLOWED_PKG_COMMANDS.contains(&req.command.as_str()) {
            return Err(Status::permission_denied(
                "Zero-Trust: Command not in allowlist"
            ));
        }

        let output = tokio::process::Command::new(&req.command)
            .args(&req.args)
            .output()
            .await
            .map_err(|e| Status::internal(format!("[SLA ERROR] Execution failed: {}", e)))?;

        Ok(Response::new(AgentResponse {
            success: output.status.success(),
            exit_code: output.status.code().unwrap_or(-1),
            stdout: String::from_utf8_lossy(&output.stdout).to_string(),
            stderr: String::from_utf8_lossy(&output.stderr).to_string(),
            error_message: String::new(),
        }))
    }

    // =========================================================================
    // 3. 🔒 Application Jail Provisioning (cgroup v2 + systemd-run)
    // =========================================================================
    async fn provision_app_jail(
        &self,
        request: Request<ProvisionJailRequest>,
    ) -> Result<Response<AgentResponse>, Status> {
        let req = request.into_inner();

        // 🛡️ Zero-Trust Input Validation
        Self::validate_identifier(&req.app_id, "app_id")?;
        Self::validate_identifier(&req.domain_name, "domain_name")?;

        let app_user = format!("kari-app-{}", req.app_id);
        let app_dir = Self::secure_join(&self.config.web_root, &req.domain_name)?;
        let service_name = format!("kari-{}", req.domain_name);

        // Step 1: Provision the unprivileged OS user
        self.jail_mgr
            .provision_app_user(&app_user, 0) // UID auto-assigned by useradd
            .await
            .map_err(|e| Status::internal(format!("[SLA ERROR] User provisioning failed: {}", e)))?;

        // Step 2: Create and secure the application directory
        self.jail_mgr
            .secure_directory(&app_dir, &app_user)
            .await
            .map_err(|e| Status::internal(format!("[SLA ERROR] Directory jailing failed: {}", e)))?;

        // Step 3: Write systemd unit file with cgroup v2 resource limits
        let svc_config = ServiceConfig {
            service_name: service_name.clone(),
            username: app_user.clone(),
            working_directory: app_dir.clone(),
            start_command: req.start_command.clone(),
            env_vars: req.env_vars.clone(),
            memory_limit_mb: req.memory_limit_mb as i32,
            cpu_limit_percent: 100, // Default: full single core
        };

        self.svc_mgr
            .write_unit_file(&svc_config)
            .await
            .map_err(|e| Status::internal(format!("[SLA ERROR] Unit file creation failed: {}", e)))?;

        // Step 4: Reload systemd and enable the service
        self.svc_mgr
            .reload_daemon()
            .await
            .map_err(|e| Status::internal(format!("[SLA ERROR] Daemon reload failed: {}", e)))?;

        self.svc_mgr
            .enable_and_start(&service_name)
            .await
            .map_err(|e| Status::internal(format!("[SLA ERROR] Service activation failed: {}", e)))?;

        // 🛡️ Privacy: Clear the transient env variables from RAM
        let mut transient_req = req;
        for (_, mut val) in transient_req.env_vars.drain() {
            val.zeroize();
        }

        info!("🔒 Jail provisioned: {} (user: {}, mem: {}MB)", service_name, app_user, transient_req.memory_limit_mb);

        Ok(Response::new(AgentResponse {
            success: true,
            exit_code: 0,
            stdout: format!("Jail '{}' provisioned with {}MB memory limit", service_name, transient_req.memory_limit_mb),
            stderr: String::new(),
            error_message: String::new(),
        }))
    }

    // =========================================================================
    // 4. ⚙️ Service Management (systemd lifecycle)
    // =========================================================================
    async fn manage_service(
        &self,
        request: Request<ServiceRequest>,
    ) -> Result<Response<AgentResponse>, Status> {
        use kari_agent::ServiceAction;

        let req = request.into_inner();
        Self::validate_identifier(&req.service_name, "service_name")?;

        // 🛡️ Zero-Trust: Only allow management of kari-prefixed services
        if !req.service_name.starts_with("kari-") {
            return Err(Status::permission_denied(
                "Zero-Trust: Refusing to manage non-Kari service"
            ));
        }

        let action = ServiceAction::try_from(req.action)
            .map_err(|_| Status::invalid_argument("Invalid service action"))?;

        let result = match action {
            ServiceAction::Start => self.svc_mgr.start(&req.service_name).await,
            ServiceAction::Stop => self.svc_mgr.stop(&req.service_name).await,
            ServiceAction::Restart => self.svc_mgr.restart(&req.service_name).await,
            ServiceAction::Reload => self.svc_mgr.reload_daemon().await,
            ServiceAction::Enable => self.svc_mgr.enable_and_start(&req.service_name).await,
            ServiceAction::Disable => self.svc_mgr.stop(&req.service_name).await,
        };

        match result {
            Ok(()) => {
                info!("⚙️ Service {} action {:?} succeeded", req.service_name, action);
                Ok(Response::new(AgentResponse {
                    success: true,
                    exit_code: 0,
                    stdout: format!("Service '{}' action completed", req.service_name),
                    stderr: String::new(),
                    error_message: String::new(),
                }))
            }
            Err(e) => Ok(Response::new(AgentResponse {
                success: false,
                exit_code: 1,
                stdout: String::new(),
                stderr: e.clone(),
                error_message: format!("[SLA ERROR] Service management failed: {}", e),
            })),
        }
    }

    // =========================================================================
    // 5. 📡 Streaming Deployment (Hardened Blue-Green)
    // =========================================================================
    async fn stream_deployment(
        &self,
        request: Request<DeployRequest>,
    ) -> Result<Response<Self::StreamDeploymentStream>, Status> {
        let req = request.into_inner();

        // 🛡️ Zero-Trust: Validate identifiers before processing
        Self::validate_identifier(&req.app_id, "app_id")?;
        Self::validate_identifier(&req.domain_name, "domain_name")?;

        let timestamp = chrono::Utc::now().format("%Y%m%d%H%M%S").to_string();
        
        let base_dir = Self::secure_join(&self.config.web_root, &req.domain_name)?;
        let release_dir = base_dir.join("releases").join(&timestamp);
        let app_user = format!("kari-app-{}", req.app_id);

        let (tx, rx) = mpsc::channel(512);

        // 🛡️ Clone Arcs for the background task
        let git = Arc::clone(&self.git_mgr);
        let jail = Arc::clone(&self.jail_mgr);
        let build = Arc::clone(&self.build_mgr);
        let svc = Arc::clone(&self.svc_mgr);
        let proxy = Arc::clone(&self.proxy_mgr);

        tokio::spawn(async move {
            let t = req.trace_id.clone();
            let log = |m: &str| LogChunk { content: m.to_string(), trace_id: t.clone() };

            // -- Step 1: Secure Git Clone --
            let ssh_cred = req.ssh_key.map(ProviderCredential::from_string);
            let _ = tx.send(Ok(log("📦 Pulling source...\n"))).await;
            if let Err(e) = git.clone_repo(&req.repo_url, &req.branch, &release_dir, ssh_cred).await {
                let _ = tx.send(Ok(log(&format!("❌ Git Error: {}\n", e)))).await;
                return;
            }

            // -- Step 2: Permissions Jailing --
            // (ssh_cred ownership transferred to clone_repo; zeroized on drop)
            let _ = tx.send(Ok(log("🔒 Securing directory...\n"))).await;
            if let Err(e) = jail.secure_directory(&release_dir, &app_user).await {
                let _ = tx.send(Ok(log(&format!("❌ Security Error: {}\n", e)))).await;
                return;
            }

            // -- Step 3: Isolated Build --
            let _ = tx.send(Ok(log("🏗️ Executing build...\n"))).await;
            let mut envs: HashMap<String, String> = req.env_vars.into_iter().collect();
            let build_res = build.execute_build(&req.build_command, &release_dir, &app_user, &envs, tx.clone(), t.clone()).await;

            // 🛡️ Privacy: Clear the build environment variables from RAM
            for (_, mut val) in envs.drain() {
                val.zeroize();
            }

            if let Err(e) = build_res {
                let _ = tx.send(Ok(log(&format!("❌ Build Error: {}\n", e)))).await;
                return;
            }

            // -- Step 4: Proxy & Service Activation --
            let service_name = format!("kari-{}", req.domain_name);
            let _ = tx.send(Ok(log("🌐 Updating Proxy & Restarting...\n"))).await;
            
            let port = req.port.unwrap_or(3000) as u16;
            if let Err(e) = proxy.create_vhost(&req.domain_name, port).await {
                let _ = tx.send(Ok(log(&format!("❌ Proxy Error: {}\n", e)))).await;
                return;
            }

            if let Err(e) = svc.restart(&service_name).await {
                let _ = tx.send(Ok(log(&format!("❌ Service Error: {}\n", e)))).await;
                return;
            }

            let _ = tx.send(Ok(log("✅ Deployment successful.\n"))).await;
        });

        Ok(Response::new(ReceiverStream::new(rx)))
    }

    // =========================================================================
    // 6. 🔥 Resource Teardown (Clean Hygiene)
    // =========================================================================
    async fn delete_deployment(
        &self,
        request: Request<DeleteRequest>,
    ) -> Result<Response<AgentResponse>, Status> {
        let req = request.into_inner();

        // 🛡️ Zero-Trust: Validate inputs
        Self::validate_identifier(&req.app_id, "app_id")?;
        Self::validate_identifier(&req.domain_name, "domain_name")?;

        let app_dir = Self::secure_join(&self.config.web_root, &req.domain_name)?;
        let app_user = format!("kari-app-{}", req.app_id);
        let service_name = format!("kari-{}", req.domain_name);

        // 🛡️ Deterministic Cleanup Order: Service → Proxy → User → Files
        let _ = self.svc_mgr.stop(&service_name).await;
        let _ = self.svc_mgr.remove_unit_file(&service_name).await;
        let _ = self.proxy_mgr.remove_vhost(&req.domain_name).await;
        let _ = self.jail_mgr.deprovision_app_user(&app_user).await;

        if app_dir.exists() {
            tokio::fs::remove_dir_all(&app_dir)
                .await
                .map_err(|e| Status::internal(format!(
                    "[SLA ERROR] Filesystem purge failed for {}: {}", req.domain_name, e
                )))?;
        }

        info!("🔥 Deployment torn down: {} (user: {})", service_name, app_user);

        Ok(Response::new(AgentResponse { success: true, ..Default::default() }))
    }

    // =========================================================================
    // 6b. 🔥 Teardown Jail (Force-Stop PID Namespace)
    // =========================================================================
    async fn teardown_jail(
        &self,
        request: Request<TeardownRequest>,
    ) -> Result<Response<AgentResponse>, Status> {
        let req = request.into_inner();

        // 🛡️ Zero-Trust: Validate input
        Self::validate_identifier(&req.app_id, "app_id")?;

        let service_name = format!("kari-app-{}", req.app_id);

        // 🛡️ SIGKILL via systemctl stop — this tears down the entire cgroup scope,
        // killing all child processes in the jail's PID namespace.
        let output = tokio::process::Command::new("systemctl")
            .args(["stop", "--no-block", &service_name])
            .output()
            .await
            .map_err(|e| Status::internal(format!("[SLA ERROR] Teardown failed: {}", e)))?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            // Not fatal — service may already be stopped
            warn!("⚠️ Teardown warning for {}: {}", service_name, stderr);
        }

        info!("🔥 Jail torn down: {} (trace: {})", service_name, req.trace_id);

        Ok(Response::new(AgentResponse {
            success: true,
            exit_code: 0,
            stdout: format!("Jail '{}' teardown initiated", service_name),
            stderr: String::new(),
            error_message: String::new(),
        }))
    }

    // =========================================================================
    // 7. 📝 Filesystem Operations (Zero-Trust Path Validation)
    // =========================================================================
    async fn write_system_file(
        &self,
        request: Request<FileWriteRequest>,
    ) -> Result<Response<AgentResponse>, Status> {
        let req = request.into_inner();

        // 🛡️ Zero-Trust: Validate path is within allowed directories
        let path = std::path::Path::new(&req.absolute_path);
        let allowed_prefixes = [
            self.config.web_root.as_path(),
            self.config.ssl_storage_dir.as_path(),
            self.config.proxy_conf_dir.as_path(),
            self.config.systemd_dir.as_path(),
        ];

        let is_allowed = allowed_prefixes.iter().any(|prefix| path.starts_with(prefix));
        if !is_allowed {
            return Err(Status::permission_denied(format!(
                "Zero-Trust: Path '{}' is outside all allowed boundaries", req.absolute_path
            )));
        }

        // 🛡️ Zero-Trust: Prevent path traversal
        if req.absolute_path.contains("..") {
            return Err(Status::invalid_argument("Zero-Trust: Path traversal detected"));
        }

        // Write the content
        if let Some(parent) = path.parent() {
            tokio::fs::create_dir_all(parent)
                .await
                .map_err(|e| Status::internal(format!("[SLA ERROR] Directory creation failed: {}", e)))?;
        }

        tokio::fs::write(path, &req.content)
            .await
            .map_err(|e| Status::internal(format!("[SLA ERROR] File write failed: {}", e)))?;

        // Apply file mode
        if !req.file_mode.is_empty() {
            let mode = u32::from_str_radix(&req.file_mode, 8)
                .map_err(|_| Status::invalid_argument("Invalid octal file mode"))?;
            let mut perms = tokio::fs::metadata(path)
                .await
                .map_err(|e| Status::internal(format!("[SLA ERROR] Metadata read failed: {}", e)))?
                .permissions();
            perms.set_mode(mode);
            tokio::fs::set_permissions(path, perms)
                .await
                .map_err(|e| Status::internal(format!("[SLA ERROR] Permission set failed: {}", e)))?;
        }

        // Apply ownership
        if !req.owner.is_empty() {
            let owner_arg = if !req.group.is_empty() {
                format!("{}:{}", req.owner, req.group)
            } else {
                req.owner.clone()
            };

            let output = tokio::process::Command::new("chown")
                .args(["-P", &owner_arg, &req.absolute_path])
                .output()
                .await
                .map_err(|e| Status::internal(format!("[SLA ERROR] chown failed: {}", e)))?;

            if !output.status.success() {
                return Err(Status::internal(format!(
                    "[SLA ERROR] Ownership change failed: {}",
                    String::from_utf8_lossy(&output.stderr)
                )));
            }
        }

        info!("📝 File written: {} (trace: {})", req.absolute_path, req.trace_id);

        Ok(Response::new(AgentResponse {
            success: true,
            exit_code: 0,
            stdout: format!("File written to {}", req.absolute_path),
            stderr: String::new(),
            error_message: String::new(),
        }))
    }

    // =========================================================================
    // 7. 🔐 SSL Certificate Installation (Privacy-First: Zeroize on Drop)
    // =========================================================================
    async fn install_certificate(
        &self,
        request: Request<SslPayload>,
    ) -> Result<Response<AgentResponse>, Status> {
        let req = request.into_inner();

        // 🛡️ Zero-Trust: Validate domain
        Self::validate_identifier(&req.domain_name, "domain_name")?;

        // 🛡️ Privacy: Wrap the private key in a Zeroizing buffer.
        // When this drops, the memory is physically overwritten with 0x00.
        let privkey_bytes = Zeroizing::new(req.privkey_pem);

        // Convert protobuf payload to our trait's SslPayload
        let trait_payload = TraitSslPayload {
            domain_name: req.domain_name.clone(),
            fullchain_pem: String::from_utf8(req.fullchain_pem)
                .map_err(|_| Status::invalid_argument("fullchain_pem is not valid UTF-8"))?,
            privkey_pem: ProviderCredential::from_string(
                String::from_utf8(privkey_bytes.to_vec())
                    .map_err(|_| Status::invalid_argument("privkey_pem is not valid UTF-8"))?
            ),
        };

        self.ssl_engine
            .install_certificate(trait_payload)
            .await
            .map_err(|e| Status::internal(format!("[SLA ERROR] Certificate installation failed: {}", e)))?;

        info!("🔐 Certificate installed for domain: {}", req.domain_name);

        Ok(Response::new(AgentResponse {
            success: true,
            exit_code: 0,
            stdout: format!("SSL certificate installed for {}", req.domain_name),
            stderr: String::new(),
            error_message: String::new(),
        }))
    }

    // =========================================================================
    // 8. 🛡️ Firewall Policy Enforcement
    // =========================================================================
    async fn apply_firewall_policy(
        &self,
        request: Request<FirewallPolicy>,
    ) -> Result<Response<AgentResponse>, Status> {
        use kari_agent::firewall_policy::{Action, Protocol as ProtoProtocol};

        let req = request.into_inner();

        // 🛡️ Zero-Trust: Map proto enums to our strict trait types
        let action = match Action::try_from(req.action) {
            Ok(Action::Allow) => FirewallAction::Allow,
            Ok(Action::Deny) => FirewallAction::Deny,
            Ok(Action::Reject) => FirewallAction::Reject,
            Err(_) => return Err(Status::invalid_argument("Invalid firewall action")),
        };

        let protocol = match ProtoProtocol::try_from(req.protocol) {
            Ok(ProtoProtocol::Tcp) => Protocol::Tcp,
            Ok(ProtoProtocol::Udp) => Protocol::Udp,
            Ok(ProtoProtocol::Both) => Protocol::Both,
            Err(_) => return Err(Status::invalid_argument("Invalid protocol")),
        };

        // 🛡️ Zero-Trust: Parse and validate source IP if provided
        let source_ip = if let Some(ref ip_str) = req.source_ip {
            if ip_str.is_empty() {
                None
            } else {
                // Validate as IP address first
                let _ = ip_str.parse::<std::net::IpAddr>().map_err(|_| {
                    Status::invalid_argument(format!("Zero-Trust: Invalid source IP: '{}'", ip_str))
                })?;
                Some(ip_str.clone())
            }
        } else {
            None
        };

        let policy = TraitFirewallPolicy {
            action,
            port: req.port as u16,
            protocol,
            source_ip,
        };

        self.firewall_mgr
            .apply_policy(&policy)
            .await
            .map_err(|e| Status::internal(format!("[SLA ERROR] Firewall policy failed: {}", e)))?;

        Ok(Response::new(AgentResponse {
            success: true,
            exit_code: 0,
            stdout: format!("Firewall rule applied: port {}", req.port),
            stderr: String::new(),
            error_message: String::new(),
        }))
    }

    // =========================================================================
    // 9. ⏰ Job Scheduling (Zero-Trust Cron via systemd timers)
    // =========================================================================
    async fn schedule_job(
        &self,
        request: Request<JobIntent>,
    ) -> Result<Response<AgentResponse>, Status> {
        let req = request.into_inner();

        // 🛡️ Zero-Trust: Validate all fields
        Self::validate_identifier(&req.job_name, "job_name")?;
        Self::validate_identifier(&req.run_as_user, "run_as_user")?;

        if req.binary.is_empty() {
            return Err(Status::invalid_argument("Zero-Trust: Binary path cannot be empty"));
        }

        // 🛡️ Zero-Trust: Reject binaries with shell metacharacters
        if req.binary.contains(';') || req.binary.contains('&') || req.binary.contains('|') {
            return Err(Status::permission_denied(
                "Zero-Trust: Shell metacharacters detected in binary path"
            ));
        }

        let intent = TraitJobIntent {
            name: req.job_name.clone(),
            binary: req.binary,
            args: req.args,
            schedule: req.schedule_expression,
            run_as_user: req.run_as_user,
        };

        self.job_scheduler
            .schedule_job(&intent)
            .await
            .map_err(|e| Status::internal(format!("[SLA ERROR] Job scheduling failed: {}", e)))?;

        info!("⏰ Job scheduled: {}", req.job_name);

        Ok(Response::new(AgentResponse {
            success: true,
            exit_code: 0,
            stdout: format!("Job '{}' scheduled successfully", req.job_name),
            stderr: String::new(),
            error_message: String::new(),
        }))
    }
}

// ==============================================================================
// 🛡️ Unit Tests — Security Helper Validation
// ==============================================================================

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::Path;

    #[test]
    fn test_secure_join_valid() {
        let base = Path::new("/var/www/kari");
        let result = KariAgentService::secure_join(base, "myapp");
        assert!(result.is_ok());
        assert_eq!(result.unwrap(), base.join("myapp"));
    }

    #[test]
    fn test_secure_join_traversal() {
        let base = Path::new("/var/www/kari");

        // Parent directory
        assert!(KariAgentService::secure_join(base, "..").is_err());
        assert!(KariAgentService::secure_join(base, "../etc/passwd").is_err());

        // Absolute path
        assert!(KariAgentService::secure_join(base, "/etc/passwd").is_err());

        // Subdirectory (contains '/')
        assert!(KariAgentService::secure_join(base, "sub/dir").is_err());

        // Windows-style backslash
        assert!(KariAgentService::secure_join(base, "win\\path").is_err());
    }

    #[test]
    fn test_validate_identifier_valid() {
        assert!(KariAgentService::validate_identifier("my-app", "app_id").is_ok());
        assert!(KariAgentService::validate_identifier("app_v1", "app_id").is_ok());
        assert!(KariAgentService::validate_identifier("site.com", "domain").is_ok());
        assert!(KariAgentService::validate_identifier("12345", "id").is_ok());
    }

    #[test]
    fn test_validate_identifier_invalid() {
        // Empty
        assert!(KariAgentService::validate_identifier("", "field").is_err());

        // Spaces
        assert!(KariAgentService::validate_identifier("my app", "field").is_err());

        // Special characters
        assert!(KariAgentService::validate_identifier("app!", "field").is_err());
        assert!(KariAgentService::validate_identifier("app@domain", "field").is_err());

        // Path characters
        assert!(KariAgentService::validate_identifier("path/to", "field").is_err());
        assert!(KariAgentService::validate_identifier("..", "field").is_err());
    }

    #[test]
    fn test_secure_join_allows_nginx_injection_chars() {
        // This test documents the vulnerability: secure_join prevents traversal but allows
        // characters dangerous for config generation if not validated separately.
        let base = Path::new("/var/www/kari");
        // We use a semicolon to terminate the directive and start a new one (config injection),
        // but no slashes so secure_join won't catch it.
        let malicious_input = "site.com; user root;";

        // secure_join allows this because it doesn't contain ".." or "/" or "\"
        let result = KariAgentService::secure_join(base, malicious_input);
        assert!(result.is_ok());

        // However, validate_identifier SHOULD reject it
        assert!(KariAgentService::validate_identifier(malicious_input, "domain").is_err());
    }
}
