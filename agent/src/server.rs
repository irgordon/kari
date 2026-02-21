// agent/src/server.rs

use std::collections::HashMap;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;
use tokio::fs;
use tokio::process::Command;
use tonic::{Request, Response, Status};

use crate::config::AgentConfig;
use crate::sys::build::{BuildManager, SystemBuildManager};
use crate::sys::cleanup::{ReleaseManager, SystemReleaseManager};
use crate::sys::git::{GitManager, SystemGitManager};
use crate::sys::jail::{JailManager, LinuxJailManager};
use crate::sys::logs::{LogManager, LinuxLogManager};
use crate::sys::systemd::{LinuxSystemdManager, ServiceConfig, ServiceManager};

pub mod kari_agent {
    tonic::include_proto!("kari.agent.v1");
}

use kari_agent::system_agent_server::SystemAgent;
use kari_agent::{
    AgentResponse, DeployRequest, FileWriteRequest, PackageRequest, ProvisionJailRequest,
    ServiceRequest,
};

fn construct_error_response(err_msg: &str) -> Result<Response<AgentResponse>, Status> {
    Ok(Response::new(AgentResponse {
        success: false,
        exit_code: -1,
        stdout: String::new(),
        stderr: err_msg.to_string(),
        error_message: err_msg.to_string(),
    }))
}

pub struct KariAgentService {
    config: AgentConfig,
    jail_mgr: Box<dyn JailManager>,
    svc_mgr: Box<dyn ServiceManager>,
    git_mgr: Box<dyn GitManager>,
    build_mgr: Box<dyn BuildManager>,
    release_mgr: Box<dyn ReleaseManager>,
    log_mgr: Box<dyn LogManager>,
}

impl KariAgentService {
    pub fn new(config: AgentConfig) -> Self {
        Self {
            jail_mgr: Box::new(LinuxJailManager),
            // Injecting paths via config
            svc_mgr: Box::new(LinuxSystemdManager::new(config.systemd_dir.clone())),
            git_mgr: Box::new(SystemGitManager),
            build_mgr: Box::new(SystemBuildManager),
            release_mgr: Box::new(SystemReleaseManager),
            log_mgr: Box::new(LinuxLogManager::new(config.logrotate_dir.clone())),
            config,
        }
    }
}

#[tonic::async_trait]
impl SystemAgent for KariAgentService {
    async fn execute_package_command(
        &self,
        request: Request<PackageRequest>,
    ) -> Result<Response<AgentResponse>, Status> {
        let req = request.into_inner();
        if req.command.is_empty() {
            return construct_error_response("Command cannot be empty");
        }

        let mut child = Command::new(&req.command);
        child.args(&req.args);

        let output = match child.output().await {
            Ok(out) => out,
            Err(e) => return construct_error_response(&format!("Failed to spawn process: {}", e)),
        };

        let exit_code = output.status.code().unwrap_or(-1);
        let success = output.status.success();

        Ok(Response::new(AgentResponse {
            success,
            exit_code,
            stdout: String::from_utf8_lossy(&output.stdout).to_string(),
            stderr: String::from_utf8_lossy(&output.stderr).to_string(),
            error_message: if success { String::new() } else { format!("Exited with code {}", exit_code) },
        }))
    }

    async fn write_system_file(
        &self,
        request: Request<FileWriteRequest>,
    ) -> Result<Response<AgentResponse>, Status> {
        let req = request.into_inner();

        if let Some(parent) = Path::new(&req.absolute_path).parent() {
            if let Err(e) = fs::create_dir_all(parent).await {
                return construct_error_response(&format!("Failed to create directories: {}", e));
            }
        }

        let tmp_path = format!("{}.tmp", req.absolute_path);
        if let Err(e) = fs::write(&tmp_path, &req.content).await {
            return construct_error_response(&format!("Failed to write temp file: {}", e));
        }

        let mode = u32::from_str_radix(&req.file_mode, 8).unwrap_or(0o644);
        let mut perms = fs::metadata(&tmp_path).await.unwrap().permissions();
        perms.set_mode(mode);
        if let Err(e) = fs::set_permissions(&tmp_path, perms).await {
            let _ = fs::remove_file(&tmp_path).await;
            return construct_error_response(&format!("Failed to set permissions: {}", e));
        }

        let chown_out = Command::new("chown").args([&format!("{}:{}", req.owner, req.group), &tmp_path]).output().await;
        if let Err(e) = chown_out {
            let _ = fs::remove_file(&tmp_path).await;
            return construct_error_response(&format!("Failed to execute chown: {}", e));
        }

        if let Err(e) = fs::rename(&tmp_path, &req.absolute_path).await {
            let _ = fs::remove_file(&tmp_path).await;
            return construct_error_response(&format!("Failed to perform atomic rename: {}", e));
        }

        Ok(Response::new(AgentResponse {
            success: true,
            exit_code: 0,
            stdout: format!("Successfully wrote {}", req.absolute_path),
            stderr: String::new(),
            error_message: String::new(),
        }))
    }

    async fn manage_service(
        &self,
        request: Request<ServiceRequest>,
    ) -> Result<Response<AgentResponse>, Status> {
        let req = request.into_inner();
        let action_str = match req.action {
            0 => "start",
            1 => "stop",
            2 => "restart",
            3 => "reload",
            4 => "enable",
            5 => "disable",
            _ => return construct_error_response("Invalid service action"),
        };

        let output = Command::new("systemctl").args([action_str, &req.service_name]).output().await;

        match output {
            Ok(out) => {
                let success = out.status.success();
                let stderr = String::from_utf8_lossy(&out.stderr).to_string();
                Ok(Response::new(AgentResponse {
                    success,
                    exit_code: out.status.code().unwrap_or(-1),
                    stdout: String::from_utf8_lossy(&out.stdout).to_string(),
                    stderr: stderr.clone(),
                    error_message: if success { String::new() } else { stderr },
                }))
            }
            Err(e) => construct_error_response(&format!("systemctl error: {}", e)),
        }
    }

    async fn provision_app_jail(
        &self,
        request: Request<ProvisionJailRequest>,
    ) -> Result<Response<AgentResponse>, Status> {
        let req = request.into_inner();
        let app_user = format!("kari-app-{}", req.app_id); 
        
        // INJECTED: Web Root
        let work_dir = format!("{}/{}", self.config.web_root, req.domain_name);
        let service_name = format!("kari-{}", req.domain_name);

        if let Err(e) = self.jail_mgr.provision_app_user(&app_user).await {
            return construct_error_response(&e);
        }
        if let Err(e) = self.jail_mgr.secure_directory(&work_dir, &app_user).await {
            return construct_error_response(&e);
        }

        let config = ServiceConfig {
            service_name: service_name.clone(),
            username: app_user,
            working_directory: work_dir,
            start_command: req.start_command,
            env_vars: req.env_vars.into_iter().collect(),
        };

        if let Err(e) = self.svc_mgr.write_unit_file(&config).await {
            return construct_error_response(&e);
        }
        if let Err(e) = self.svc_mgr.reload_daemon().await {
            return construct_error_response(&e);
        }
        if let Err(e) = self.svc_mgr.enable_and_start(&service_name).await {
            return construct_error_response(&e);
        }

        Ok(Response::new(AgentResponse {
            success: true,
            exit_code: 0,
            stdout: "App successfully isolated and started via systemd".to_string(),
            stderr: String::new(),
            error_message: String::new(),
        }))
    }

    async fn deploy_application(
        &self,
        request: Request<DeployRequest>,
    ) -> Result<Response<AgentResponse>, Status> {
        let req = request.into_inner();
        let timestamp = chrono::Utc::now().format("%Y%m%d%H%M%S").to_string();
        
        // INJECTED: Web Root
        let base_dir = format!("{}/{}", self.config.web_root, req.domain_name);
        
        let release_dir = format!("{}/releases/{}", base_dir, timestamp);
        let current_symlink = format!("{}/current", base_dir);
        let app_user = format!("kari-app-{}", req.app_id);

        let (log_tx, mut log_rx) = tokio::sync::mpsc::channel(100);
        let mut full_build_log = String::new();

        let log_collector = tokio::spawn(async move {
            let mut logs = String::new();
            while let Some(line) = log_rx.recv().await {
                logs.push_str(&line);
            }
            logs
        });

        let _ = log_tx.send("Starting Git Clone...\n".to_string()).await;
        if let Err(e) = self.git_mgr.clone_repo(&req.repo_url, &req.branch, &release_dir).await {
            return construct_error_response(&format!("Git Clone Failed: {}", e)); 
        }

        let _ = self.jail_mgr.secure_directory(&release_dir, &app_user).await;

        let _ = log_tx.send("Starting Isolated Build Process...\n".to_string()).await;
        let env_map: HashMap<String, String> = req.env_vars.into_iter().collect();
        
        if let Err(e) = self.build_mgr.execute_build(&req.build_command, &release_dir, &app_user, &env_map, log_tx.clone()).await {
            let _ = fs::remove_dir_all(&release_dir).await;
            return construct_error_response(&format!("Build failed: {}", e));
        }

        let _ = log_tx.send("Build successful. Performing atomic swap...\n".to_string()).await;
        let ln_out = Command::new("ln").args(["-sfn", &release_dir, &current_symlink]).output().await.map_err(|e| e.to_string()).unwrap();

        if !ln_out.status.success() {
            return construct_error_response("Failed to update symlink");
        }

        let releases_dir = format!("{}/releases", base_dir);
        match self.release_mgr.prune_old_releases(&releases_dir, 5).await {
            Ok(deleted) if deleted > 0 => { let _ = log_tx.send(format!("Cleaned up {} old release(s).\n", deleted)).await; }
            Err(e) => { let _ = log_tx.send(format!("Warning: Cleanup failed: {}\n", e)).await; }
            _ => {} 
        }

        let log_dir = format!("{}/logs", base_dir);
        let _ = self.jail_mgr.secure_directory(&log_dir, &app_user).await; 
        if let Err(e) = self.log_mgr.configure_logrotate(&req.domain_name, &log_dir).await {
            let _ = log_tx.send(format!("Warning: Logrotate config failed: {}\n", e)).await;
        }

        let service_name = format!("kari-{}", req.domain_name);
        let _ = self.svc_mgr.reload_daemon().await; 
        let _ = Command::new("systemctl").args(["restart", &service_name]).output().await;

        drop(log_tx);
        if let Ok(collected_logs) = log_collector.await {
            full_build_log = collected_logs;
        }

        Ok(Response::new(AgentResponse {
            success: true,
            exit_code: 0,
            stdout: full_build_log,
            stderr: "".to_string(),
            error_message: "".to_string(),
        }))
    }
}
