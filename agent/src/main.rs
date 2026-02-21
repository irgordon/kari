// agent/src/main.rs

use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;
use tokio::net::UnixListener;
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
    // 1. Configuration & Environment (Platform Agnostic)
    // ==============================================================================
    
    // Initialize structured logging
    tracing_subscriber::fmt::init();
    let config = AgentConfig::load();
    
    // SLA / Agnosticism: We dynamically inject the path instead of hardcoding it.
    let socket_path = config.socket_path.clone(); 
    let socket_dir = Path::new(&socket_path).parent().unwrap();

    // ==============================================================================
    // 2. Secure Socket Initialization
    // ==============================================================================

    // Ensure the runtime directory exists
    if !socket_dir.exists() {
        fs::create_dir_all(socket_dir)?;
    }

    // Clean up existing socket file if it exists from a previous crash/run
    if Path::new(&socket_path).exists() {
        fs::remove_file(&socket_path)?;
    }

    // Bind to the Unix Domain Socket
    let listener = UnixListener::bind(&socket_path)?;
    
    // 🛡️ DEFENSE IN DEPTH: Restrict socket permissions
    // 0o660 (rw-rw----) prevents unauthorized users from even opening the file.
    let mut perms = fs::metadata(&socket_path)?.permissions();
    perms.set_mode(0o660);
    fs::set_permissions(&socket_path, perms)?;

    // ==============================================================================
    // 3. SLA Boundary: Kernel-Level Peer Credential Interceptor
    // ==============================================================================
    
    let expected_api_uid = config.expected_api_uid;

    // We replace UnixListenerStream with a custom stream that verifies identity 
    // *before* handing the connection off to the Tonic gRPC server.
    let incoming_stream = async_stream::stream! {
        loop {
            match listener.accept().await {
                Ok((stream, _)) => {
                    match stream.peer_cred() {
                        Ok(cred) => {
                            // Enforce Zero-Trust: Only allow the Go API's exact UID or Root (0)
                            if cred.uid() == expected_api_uid || cred.uid() == 0 {
                                tracing::debug!("✅ Authenticated gRPC connection from UID: {}", cred.uid());
                                yield Ok::<_, std::io::Error>(stream);
                            } else {
                                // SLA Violation: Immediately drop the connection.
                                tracing::warn!(
                                    "🚨 BLOCKED unauthorized socket connection attempt from UID: {} / GID: {}", 
                                    cred.uid(), cred.gid()
                                );
                            }
                        }
                        Err(e) => tracing::error!("Failed to read peer credentials: {}", e),
                    }
                }
                Err(e) => {
                    tracing::error!("Socket accept error: {}", e);
                    yield Err(e);
                }
            }
        }
    };

    // ==============================================================================
    // 4. Dependency Injection & Service Start
    // ==============================================================================

    // Instantiate the orchestrator with our dynamic configuration
    // This fulfills the SOLID Open/Closed principle.
    let agent_service = KariAgentService::new(config);

    tracing::info!("⚙️ Kari Rust Agent (The Muscle) securely listening on {}", socket_path);

    Server::builder()
        .add_service(SystemAgentServer::new(agent_service))
        .serve_with_incoming(incoming_stream)
        .await?;

    Ok(())
}
