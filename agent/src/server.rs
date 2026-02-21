use std::collections::HashMap;
use std::sync::Arc;
use std::path::Path;
use tokio::sync::mpsc;
use tokio_stream::wrappers::ReceiverStream;
use tonic::{Request, Response, Status};
use tracing::{info, warn, error};

use crate::config::AgentConfig;
use crate::sys::build::{BuildManager, SystemBuildManager};
use crate::sys::git::{GitManager, SystemGitManager};
use crate::sys::jail::{JailManager, LinuxJailManager};
use crate::sys::systemd::{LinuxSystemdManager, ServiceManager};
use crate::sys::traits::ProxyManager; // 🛡️ Added
use crate::sys::secrets::ProviderCredential; // 🛡️ Added

// Import the generated gRPC types
pub mod kari_agent {
    tonic::include_proto!("kari.agent.v1");
}

use kari_agent::system_agent_server::SystemAgent;
use kari_agent::{
    AgentResponse, DeployRequest, PackageRequest, 
    ServiceRequest, LogChunk, DeleteRequest,
};

const ALLOWED_PKG_COMMANDS: &[&str] = &["apt-get", "apt", "dnf", "yum", "zypper"];

pub struct KariAgentService {
    config: AgentConfig,
    jail_mgr: Arc<dyn JailManager>,
    svc_mgr: Arc<dyn ServiceManager>,
    git_mgr: Arc<dyn GitManager>,
    build_mgr: Arc<dyn BuildManager>,
    proxy_mgr: Arc<dyn ProxyManager>, // 🛡️ Multi-platform Proxy support
}

impl KariAgentService {
    pub fn new(config: AgentConfig, proxy_mgr: Arc<dyn ProxyManager>) -> Self {
        Self {
            jail_mgr: Arc::new(LinuxJailManager),
            svc_mgr: Arc::new(LinuxSystemdManager::new(config.systemd_dir.clone())),
            git_mgr: Arc::new(SystemGitManager),
            build_mgr: Arc::new(SystemBuildManager),
            proxy_mgr,
            config,
        }
    }

    /// 🛡️ Zero-Trust: Strictly prevents directory traversal
    fn secure_join(&self, base: &Path, unsafe_suffix: &str) -> Result<std::path::PathBuf, Status> {
        if unsafe_suffix.contains("..") || unsafe_suffix.contains('/') || unsafe_suffix.contains('\\') {
            return Err(Status::invalid_argument("Path traversal detected in identifier"));
        }
        Ok(base.join(unsafe_suffix))
    }
}

#[tonic::async_trait]
impl SystemAgent for KariAgentService {
    type StreamDeploymentStream = ReceiverStream<Result<LogChunk, Status>>;

    // --- 1. Package Management (Hardened) ---
    async fn execute_package_command(
        &self,
        request: Request<PackageRequest>,
    ) -> Result<Response<AgentResponse>, Status> {
        let req = request.into_inner();
        
        if !ALLOWED_PKG_COMMANDS.contains(&req.command.as_str()) {
            return Err(Status::permission_denied("Command not whitelisted"));
        }

        let output = tokio::process::Command::new(&req.command)
            .args(&req.args)
            .output()
            .await
            .map_err(|e| Status::internal(format!("Execution failed: {}", e)))?;

        Ok(Response::new(AgentResponse {
            success: output.status.success(),
            exit_code: output.status.code().unwrap_or(-1),
            stdout: String::from_utf8_lossy(&output.stdout).to_string(),
            stderr: String::from_utf8_lossy(&output.stderr).to_string(),
            error_message: String::new(),
        }))
    }

    // --- 2. Resource Teardown (Clean Hygiene) ---
    async fn delete_deployment(
        &self,
        request: Request<DeleteRequest>,
    ) -> Result<Response<AgentResponse>, Status> {
        let req = request.into_inner();
        let app_dir = self.secure_join(&self.config.web_root, &req.domain_name)?;
        let app_user = format!("kari-app-{}", req.app_id);
        let service_name = format!("kari-{}", req.domain_name);

        // 🛡️ Deterministic Cleanup: Service -> Proxy -> User -> Files
        let _ = self.svc_mgr.stop(&service_name).await;
        let _ = self.svc_mgr.remove_unit_file(&service_name).await;
        let _ = self.proxy_mgr.remove_vhost(&req.domain_name).await;
        let _ = self.jail_mgr.deprovision_app_user(&app_user).await;
        
        tokio::fs::remove_dir_all(&app_dir).await
            .map_err(|e| Status::internal(format!("Filesystem purge failed: {}", e)))?;

        Ok(Response::new(AgentResponse { success: true, ..Default::default() }))
    }

    // --- 3. Streaming Deployment (Hardened Blue-Green) ---
    async fn stream_deployment(
        &self,
        request: Request<DeployRequest>,
    ) -> Result<Response<Self::StreamDeploymentStream>, Status> {
        let req = request.into_inner();
        let timestamp = chrono::Utc::now().format("%Y%m%d%H%M%S").to_string();
        
        let base_dir = self.secure_join(&self.config.web_root, &req.domain_name)?;
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
            let _ = tx.send(Ok(log("🔒 Securing directory...\n"))).await;
            if let Err(e) = jail.secure_directory(&release_dir, &app_user).await {
                let _ = tx.send(Ok(log(&format!("❌ Security Error: {}\n", e)))).await;
                return;
            }

            // -- Step 3: Isolated Build --
            let _ = tx.send(Ok(log("🏗️ Executing build...\n"))).await;
            let envs: HashMap<String, String> = req.env_vars.into_iter().collect();
            if let Err(e) = build.execute_build(&req.build_command, &release_dir, &app_user, &envs, tx.clone(), t.clone()).await {
                let _ = tx.send(Ok(log(&format!("❌ Build Error: {}\n", e)))).await;
                return;
            }

            // -- Step 4: Proxy & Service Activation --
            let service_name = format!("kari-{}", req.domain_name);
            let _ = tx.send(Ok(log("🌐 Updating Proxy & Restarting...\n"))).await;
            
            // Assume the app's internal port is provided in req.port
            let port = req.port as u16;
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
}
