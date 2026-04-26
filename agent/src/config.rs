use std::env;
use std::path::PathBuf;

#[derive(Clone, Debug)]
pub struct AgentConfig {
    // 🛡️ SLA Boundary: Network & Identity
    pub socket_path: PathBuf,
    pub expected_api_uid: u32,
    pub expected_api_gid: u32,

    // 📂 Platform Agnostic Paths (Strictly Typed)
    pub web_root: PathBuf,
    pub systemd_dir: PathBuf,
    pub logrotate_dir: PathBuf,
    pub ssl_storage_dir: PathBuf,
    pub proxy_conf_dir: PathBuf,
}

impl AgentConfig {
    pub fn load() -> Self {
        // 1. 🛡️ Zero-Trust Identity Parsing (No Default Guesses!)
        // We explicitly remove the `unwrap_or_else` fallback. The deployment
        // environment MUST explicitly state the UID of the Go Brain.
        // If it's missing, the Muscle refuses to boot to prevent unauthorized access.
        let expected_api_uid = env::var("KARI_API_UID")
            .expect("🚨 SECURITY FATAL: KARI_API_UID environment variable is strictly required")
            .parse::<u32>()
            .expect("🚨 SECURITY FATAL: KARI_API_UID must be a valid numeric User ID");

        let expected_api_gid = env::var("KARI_API_GID")
            .expect("🚨 SECURITY FATAL: KARI_API_GID environment variable is strictly required")
            .parse::<u32>()
            .expect("🚨 SECURITY FATAL: KARI_API_GID must be a valid numeric Group ID");

        Self {
            socket_path: PathBuf::from(
                env::var("KARI_SOCKET_PATH")
                    .unwrap_or_else(|_| "/var/run/kari/agent.sock".to_string()),
            ),

            expected_api_uid,
            expected_api_gid,

            // 2. 🛡️ Type-Safe File System Boundaries
            web_root: PathBuf::from(
                env::var("KARI_WEB_ROOT").unwrap_or_else(|_| "/var/www/kari".to_string()),
            ),

            systemd_dir: PathBuf::from(
                env::var("KARI_SYSTEMD_DIR").unwrap_or_else(|_| "/etc/systemd/system".to_string()),
            ),

            logrotate_dir: PathBuf::from(
                env::var("KARI_LOGROTATE_DIR").unwrap_or_else(|_| "/etc/logrotate.d".to_string()),
            ),

            ssl_storage_dir: PathBuf::from(
                env::var("KARI_SSL_DIR").unwrap_or_else(|_| "/etc/kari/ssl".to_string()),
            ),

            proxy_conf_dir: PathBuf::from(
                env::var("KARI_PROXY_CONF_DIR")
                    .unwrap_or_else(|_| "/etc/nginx/sites-available".to_string()),
            ),
        }
    }
}
