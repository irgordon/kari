use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use tokio::net::UnixListener;
use tokio::signal;
use tonic::transport::Server;
use tracing::{debug, error, info, warn};

mod config;
mod server;
mod sys;

use crate::config::AgentConfig;
use crate::server::KariAgentService;
use crate::server::kari_agent::system_agent_server::SystemAgentServer;

// 🛡️ SOLID: Import trait types for discovery, concrete types for construction
use crate::sys::firewall::LinuxFirewallManager;
use crate::sys::proxy::{ApacheManager, NginxManager};
use crate::sys::scheduler::SystemdTimerManager;
use crate::sys::ssl::LinuxSslEngine;
use crate::sys::traits::ProxyManager;

/// 🛡️ SLA: Automatic Proxy Discovery
/// Probes the host system to determine the available ingress controller.
fn discover_proxy_manager() -> Result<Arc<dyn ProxyManager>, Box<dyn std::error::Error>> {
    // 1. Check for Nginx (Primary 2026 Choice)
    if Path::new("/etc/nginx/sites-available").exists() {
        info!("🔍 Discovery: Nginx detected. Initializing NginxProxyManager...");
        return Ok(Arc::new(NginxManager::new(PathBuf::from("/etc/nginx"))));
    }

    // 2. Check for Apache (Legacy/Standard Choice)
    if Path::new("/etc/apache2/sites-available").exists() {
        info!("🔍 Discovery: Apache2 detected. Initializing ApacheManager...");
        return Ok(Arc::new(ApacheManager::new(PathBuf::from("/etc/apache2"))));
    }

    Err("SLA FAILURE: No supported Proxy Manager (Nginx/Apache) found on this host.".into())
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // 1. Core Telemetry
    tracing_subscriber::fmt::init();
    info!("🚀 Karı Rust Agent (The Muscle) v2026.1 initializing...");

    let config = AgentConfig::load();
    let socket_path = PathBuf::from(&config.socket_path);

    // 🛡️ Zero-Trust: Safe parent resolution
    let socket_dir = socket_path
        .parent()
        .ok_or("Invalid socket path: no parent directory")?;

    // 2. Filesystem Preparation
    if !socket_dir.exists() {
        fs::create_dir_all(socket_dir)?;
    }
    if socket_path.exists() {
        debug!("Cleaning up stale socket at {:?}", socket_path);
        fs::remove_file(&socket_path)?;
    }

    // 3. 🛡️ SOLID: Dependency Discovery & Injection
    // Each manager is discovered/constructed BEFORE the socket binds.
    // If the host isn't ready, the Muscle refuses to start.
    let proxy_mgr = discover_proxy_manager()?;
    let firewall_mgr = Arc::new(LinuxFirewallManager::new());
    let ssl_engine = Arc::new(LinuxSslEngine::new(config.ssl_storage_dir.clone()));
    let job_scheduler = Arc::new(SystemdTimerManager::new(
        config.systemd_dir.to_string_lossy().to_string(),
    ));

    // 4. Bind and Secure the Socket
    let listener = UnixListener::bind(&socket_path)?;

    let mut perms = fs::metadata(&socket_path)?.permissions();
    perms.set_mode(0o660); // rw-rw----
    fs::set_permissions(&socket_path, perms)?;

    // 🛡️ Kernel-Level Handover (SO_PEERCRED Pre-requisite)
    let uid = config.expected_api_uid;
    let gid = config.expected_api_gid;
    nix::unistd::chown(
        &socket_path,
        Some(nix::unistd::Uid::from_raw(uid)),
        Some(nix::unistd::Gid::from_raw(gid)),
    )
    .map_err(|e| format!("SLA Failure: Failed to chown socket: {}", e))?;

    // 5. Peer Credential Guard (Kernel-Level Auth)
    let incoming_stream = async_stream::stream! {
        loop {
            match listener.accept().await {
                Ok((stream, _)) => {
                    if let Ok(cred) = stream.peer_cred() {
                        // 🛡️ Zero-Trust: Only the Go API User or Root can talk to this socket
                        if cred.uid() == uid || cred.uid() == 0 {
                            debug!("✅ Verified connection: UID {}", cred.uid());
                            yield Ok::<_, std::io::Error>(stream);
                        } else {
                            warn!("🚨 SECURITY ALERT: Unauthorized connection from UID {}", cred.uid());
                        }
                    }
                }
                Err(e) => {
                    error!("Socket accept failure: {}", e);
                    yield Err(e);
                }
            }
        }
    };

    // 6. Start the Service
    let agent_service =
        KariAgentService::new(config, proxy_mgr, firewall_mgr, ssl_engine, job_scheduler);
    let grpc_server = Server::builder()
        .add_service(SystemAgentServer::new(agent_service))
        .serve_with_incoming(incoming_stream);

    info!(
        "⚙️ Agent listening on {:?} [Target UID: {}]",
        socket_path, uid
    );

    // 7. Graceful Shutdown
    tokio::select! {
        res = grpc_server => {
            if let Err(e) = res {
                error!("CRITICAL: Server crashed: {}", e);
            }
        }
        _ = signal::ctrl_c() => {
            info!("🛑 Shutdown signal received. Cleaning up...");
        }
    }

    if socket_path.exists() {
        let _ = fs::remove_file(socket_path);
    }
    info!("👋 Karı Muscle shutdown complete.");

    Ok(())
}
