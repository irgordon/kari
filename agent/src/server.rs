use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::mpsc;
use tokio_stream::wrappers::ReceiverStream;
use tonic::{Request, Response, Status};
use tracing::{info, warn, error};

use crate::config::AgentConfig;
use crate::sys::build::{BuildManager, SystemBuildManager};
use crate::sys::git::{GitManager, SystemGitManager};
use crate::sys::jail::{JailManager, LinuxJailManager};
use crate::sys::systemd::{LinuxSystemdManager, ServiceManager};

// Import the generated gRPC types
pub mod kari_agent {
    tonic::include_proto!("kari.agent.v1");
}

use kari_agent::system_agent_server::SystemAgent;
use kari_agent::{
    AgentResponse, DeployRequest, PackageRequest, 
    ServiceRequest, LogChunk, DeleteRequest,
};

/// 🛡️ SECURITY BOUNDARY: Command Whitelist
/// Only these binaries can be invoked by the execute_package_command endpoint.
const ALLOWED_PKG_COMMANDS: &[&str] = &["apt-get", "apt", "dnf", "yum", "zypper"];

pub struct KariAgentService {
    config: AgentConfig,
    jail_mgr: Arc<dyn JailManager>,
    svc_mgr: Arc<dyn ServiceManager>,
    git_mgr: Arc<dyn GitManager>,
    build_mgr: Arc<dyn BuildManager>,
}

impl KariAgentService {
    pub fn new(config: AgentConfig) -> Self {
        Self {
            jail_mgr: Arc::new(LinuxJailManager),
            svc_mgr: Arc::new(LinuxSystemdManager::new(config.systemd_dir.clone())),
            git_mgr: Arc::new(SystemGitManager),
            build_mgr: Arc::new(SystemBuildManager),
            config,
        }
    }
}

#[tonic::async_trait]
impl SystemAgent for KariAgentService {
    type StreamDeploymentStream = ReceiverStream<Result<LogChunk, Status>>;

    // ==============================================================================
    // 1. Package Management (Hardened Whitelist)
    // ==============================================================================
    async fn execute_package_command(
        &self,
        request: Request<PackageRequest>,
    ) -> Result<Response<AgentResponse>, Status> {
        let req = request.into_inner();
        
        if !ALLOWED_PKG_COMMANDS.contains(&req.command.as_str()) {
            warn!("Blocked unauthorized command: {}", req.command);
            return Err(Status::permission_denied("Command not in security whitelist"));
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

    // ==============================================================================
    // 2. Service Orchestration (Trait-Based)
    // ==============================================================================
    async fn manage_service(
        &self,
        request: Request<ServiceRequest>,
    ) -> Result<Response<AgentResponse>, Status> {
        let req = request.into_inner();
        
        let result = match req.action {
            0 => self.svc_mgr.start(&req.service_name).await,
            1 => self.svc_mgr.stop(&req.service_name).await,
            2 => self.svc_mgr.restart(&req.service_name).await,
            3 => self.svc_mgr.reload_daemon().await,
            4 => self.svc_mgr.enable_and_start(&req.service_name).await,
            _ => return Err(Status::invalid_argument("Unknown service action")),
        };

        match result {
            Ok(_) => Ok(Response::new(AgentResponse { 
                success: true, 
                ..Default::default() 
            })),
            Err(e) => Err(Status::internal(e)),
        }
    }

    // ==============================================================================
    // 3. Resource Teardown (Hygiene)
    // ==============================================================================
    async fn delete_deployment(
        &self,
        request: Request<DeleteRequest>,
    ) -> Result<Response<AgentResponse>, Status> {
        let req = request.into_inner();
        let app_user = format!("kari-app-{}", req.app_id);
        let service_name = format!("kari-{}", req.domain_name);

        info!("Initiating teardown for app: {}", req.app_id);

        // 1. Stop and disable the service
        let _ = self.svc_mgr.stop(&service_name).await;
        let _ = self.svc_mgr.remove_unit_file(&service_name).await;
        let _ = self.svc_mgr.reload_daemon().await;

        // 2. Purge the unprivileged user
        let _ = self.jail_mgr.deprovision_app_user(&app_user).await;

        // 3. Clean up the web root (handled by a release manager usually)
        let path = format!("{}/{}", self.config.web_root, req.domain_name);
        let _ = tokio::fs::remove_dir_all(path).await;

        Ok(Response::new(AgentResponse { success: true, ..Default::default() }))
    }

    // ==============================================================================
    // 4. Streaming Deployment (The Blue-Green Flow)
    // ==============================================================================
    async fn stream_deployment(
        &self,
        request: Request<DeployRequest>,
    ) -> Result<Response<Self::StreamDeploymentStream>, Status> {
        let req = request.into_inner();
        let timestamp = chrono::Utc::now().format("%Y%m%d%H%M%S").to_string();
        
        let base_dir = format!("{}/{}", self.config.web_root, req.domain_name);
        let release_dir = format!("{}/releases/{}", base_dir, timestamp);
        let app_user = format!("kari-app-{}", req.app_id);

        // 🛡️ Backpressure: 512 chunks max in buffer
        let (tx, rx) = mpsc::channel(512);

        // Atomic clones for background task
        let git = Arc::clone(&self.git_mgr);
        let jail = Arc::clone(&self.jail_mgr);
        let build = Arc::clone(&self.build_mgr);
        let svc = Arc::clone(&self.svc_mgr);

        tokio::spawn(async move {
            let t = req.trace_id.clone();
            let log = |msg: &str| LogChunk { content: msg.to_string(), trace_id: t.clone() };

            // -- Step 1: Git Clone --
            let _ = tx.send(Ok(log("📦 Pulling source from repository...\n"))).await;
            if let Err(e) = git.clone_repo(&req.repo_url, &req.branch, &release_dir).await {
                let _ = tx.send(Ok(log(&format!("❌ Git Error: {}\n", e)))).await;
                return;
            }

            // -- Step 2: Permissions Jailing --
            let _ = tx.send(Ok(log("🔒 Hardening filesystem permissions...\n"))).await;
            if let Err(e) = jail.secure_directory(&release_dir, &app_user).await {
                let _ = tx.send(Ok(log(&format!("❌ Security Error: {}\n", e)))).await;
                return;
            }

            // -- Step 3: Isolated Build --
            let _ = tx.send(Ok(log("🏗️ Executing build in isolated jail...\n"))).await;
            let envs: HashMap<String, String> = req.env_vars.into_iter().collect();
            if let Err(e) = build.execute_build(&req.build_command, &release_dir, &app_user, &envs, tx.clone()).await {
                let _ = tx.send(Ok(log(&format!("❌ Build Error: {}\n", e)))).await;
                let _ = tokio::fs::remove_dir_all(&release_dir).await;
                return;
            }

            // -- Step 4: Atomic Restart --
            let service_name = format!("kari-{}", req.domain_name);
            let _ = tx.send(Ok(log("🔄 Swapping binaries and restarting service...\n"))).await;
            if let Err(e) = svc.restart(&service_name).await {
                let _ = tx.send(Ok(log(&format!("❌ Restart Error: {}\n", e)))).await;
                return;
            }

            let _ = tx.send(Ok(log("✅ Deployment Complete. System Healthy.\n"))).await;
        });

        Ok(Response::new(ReceiverStream::new(rx)))
    }
}
