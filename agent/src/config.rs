// agent/src/config.rs

use std::env;

#[derive(Clone, Debug)]
pub struct AgentConfig {
    // 🛡️ SLA Boundary: Network & Identity
    pub socket_path: String,
    pub expected_api_uid: u32,
    
    // 📂 Platform Agnostic Paths
    pub web_root: String,
    pub systemd_dir: String,
    pub logrotate_dir: String,
    pub ssl_storage_dir: String,
}

impl AgentConfig {
    pub fn load() -> Self {
        // 🛡️ Zero-Trust Identity Parsing
        // We strictly parse the UID as an integer. If the admin provides a non-numeric 
        // string in the environment variable, the Agent refuses to start, preventing 
        // a bypassed SO_PEERCRED check. Defaults to 1001 (standard for first system user).
        let expected_api_uid = env::var("KARI_API_UID")
            .unwrap_or_else(|_| "1001".to_string())
            .parse::<u32>()
            .expect("SECURITY FATAL: KARI_API_UID must be a valid numeric User ID");

        Self {
            socket_path: env::var("KARI_SOCKET_PATH")
                .unwrap_or_else(|_| "/var/run/kari/agent.sock".to_string()),
            
            expected_api_uid,
            
            // Scoped securely to a Kari-specific subfolder to prevent collision
            web_root: env::var("KARI_WEB_ROOT")
                .unwrap_or_else(|_| "/var/www/kari".to_string()),
                
            systemd_dir: env::var("KARI_SYSTEMD_DIR")
                .unwrap_or_else(|_| "/etc/systemd/system".to_string()),
                
            logrotate_dir: env::var("KARI_LOGROTATE_DIR")
                .unwrap_or_else(|_| "/etc/logrotate.d".to_string()),
                
            ssl_storage_dir: env::var("KARI_SSL_DIR")
                .unwrap_or_else(|_| "/etc/kari/ssl".to_string()),
        }
    }
}
