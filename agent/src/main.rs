// agent/src/main.rs

use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;
use tokio::net::UnixListener;
use tokio_stream::wrappers::UnixListenerStream;
use tonic::transport::Server;

mod config;
mod server;
mod sys;

use crate::config::AgentConfig;
use crate::server::kari_agent::system_agent_server::SystemAgentServer;
use crate::server::KariAgentService;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // ==============================================================================
    // 1. Configuration & Environment (SLA Layer)
    // ==============================================================================
    
    // Initialize structured logging
    tracing_subscriber::fmt::init();
    let config = AgentConfig::load();
    
    // Define the secure socket path
    // Platform Agnostic: In production, this is usually /var/run/kari/agent.sock
    let socket_path = "/var/run/kari/agent.sock";
    let socket_dir = Path::new(socket_path).parent().unwrap();

    // ==============================================================================
    // 2. Secure Socket Initialization
    // ==============================================================================

    // Ensure the runtime directory exists
    if !socket_dir.exists() {
        fs::create_dir_all(socket_dir)?;
    }

    // Clean up existing socket file if it exists from a previous crash/run
    if Path::new(socket_path).exists() {
        fs::remove_file(socket_path)?;
    }

    // Bind to the Unix Domain Socket
    let uds = UnixListener::bind(socket_path)?;
    
    // 🛡️ SECURITY BOUNDARY: Restrict socket permissions
    // 0o660 (rw-rw----) allows the root owner (Agent) and the group (which the Go API belongs to)
    // to communicate, while denying all other users on the system.
    let mut perms = fs::metadata(socket_path)?.permissions();
    perms.set_mode(0o660);
    fs::set_permissions(socket_path, perms)?;

    let uds_stream = UnixListenerStream::new(uds);

    // ==============================================================================
    // 3. Dependency Injection & Service Start
    // ==============================================================================

    // Instantiate the orchestrator with our dynamic configuration
    // This fulfills the SOLID Open/Closed principle: we can swap implementations 
    // in KariAgentService without touching the main server loop.
    let agent_service = KariAgentService::new(config);

    tracing::info!("⚙️ Kari Rust Agent (The Muscle) starting on {}", socket_path);

    Server::builder()
        .add_service(SystemAgentServer::new(agent_service))
        .serve_with_incoming(uds_stream)
        .await?;

    Ok(())
}
