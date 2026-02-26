use async_trait::async_trait;
use tokio::fs;
use tokio::process::Command;
use std::path::PathBuf;
use crate::sys::traits::ProxyManager;

/// 🛡️ Zero-Trust: Strictly validates domain names to prevent config injection
fn validate_domain_format(domain: &str) -> Result<(), String> {
    if domain.is_empty() {
        return Err("Domain cannot be empty".to_string());
    }
    if domain.contains("..") || domain.contains('/') || domain.contains('\\') {
        return Err(format!("Zero-Trust: Path traversal detected in domain: '{}'", domain));
    }
    // Allow alphanumeric, dots, hyphens, underscores.
    // Reject everything else (including spaces, quotes, brackets, semicolons)
    if !domain.chars().all(|c| c.is_ascii_alphanumeric() || c == '.' || c == '-' || c == '_') {
        return Err(format!("Zero-Trust: Invalid characters in domain name: '{}'", domain));
    }
    Ok(())
}

// ==============================================================================
// 1. Apache Implementation
// ==============================================================================
pub struct ApacheManager {
    base_path: PathBuf,
}

impl ApacheManager {
    pub fn new(base_path: PathBuf) -> Self {
        Self { base_path }
    }

    async fn test_and_reload(&self) -> Result<(), String> {
        let check = Command::new("apache2ctl").arg("configtest").output().await
            .map_err(|e| format!("Apache check failed: {}", e))?;

        if !check.status.success() {
            return Err(format!("Apache config error: {}", String::from_utf8_lossy(&check.stderr)));
        }

        Command::new("systemctl").args(["reload", "apache2"]).output().await
            .map_err(|e| format!("Systemd reload failed: {}", e))?;
        Ok(())
    }
}

#[async_trait]
impl ProxyManager for ApacheManager {
    async fn create_vhost(&self, domain: &str, target_port: u16) -> Result<(), String> {
        validate_domain_format(domain)?;

        let config_path = self.base_path.join("sites-available").join(format!("{}.conf", domain));
        let enabled_link = self.base_path.join("sites-enabled").join(format!("{}.conf", domain));

        let content = format!(
            r#"<VirtualHost *:80>
    ServerName {domain}
    ProxyPreserveHost On
    ProxyPass / http://127.0.0.1:{target_port}/
    ProxyPassReverse / http://127.0.0.1:{target_port}/
    Header always set X-Content-Type-Options "nosniff"
</VirtualHost>"#,
            domain = domain, target_port = target_port
        );

        fs::write(&config_path, content).await.map_err(|e| e.to_string())?;
        if !enabled_link.exists() {
            fs::symlink(&config_path, &enabled_link).await.map_err(|e| e.to_string())?;
        }
        self.test_and_reload().await
    }

    async fn remove_vhost(&self, domain: &str) -> Result<(), String> {
        validate_domain_format(domain)?;

        let config_path = self.base_path.join("sites-available").join(format!("{}.conf", domain));
        let enabled_link = self.base_path.join("sites-enabled").join(format!("{}.conf", domain));
        let _ = fs::remove_file(enabled_link).await;
        let _ = fs::remove_file(config_path).await;
        self.test_and_reload().await
    }
}

// ==============================================================================
// 2. Nginx Implementation
// ==============================================================================
pub struct NginxManager {
    base_path: PathBuf,
}

impl NginxManager {
    pub fn new(base_path: PathBuf) -> Self {
        Self { base_path }
    }

    async fn test_and_reload(&self) -> Result<(), String> {
        let check = Command::new("nginx").arg("-t").output().await
            .map_err(|e| format!("Nginx check failed: {}", e))?;

        if !check.status.success() {
            return Err(format!("Nginx config error: {}", String::from_utf8_lossy(&check.stderr)));
        }

        Command::new("systemctl").args(["reload", "nginx"]).output().await
            .map_err(|e| format!("Systemd reload failed: {}", e))?;
        Ok(())
    }
}

#[async_trait]
impl ProxyManager for NginxManager {
    async fn create_vhost(&self, domain: &str, target_port: u16) -> Result<(), String> {
        validate_domain_format(domain)?;

        let config_path = self.base_path.join("sites-available").join(domain);
        let enabled_link = self.base_path.join("sites-enabled").join(domain);

        let content = format!(
            r#"server {{
    listen 80;
    server_name {domain};

    location / {{
        proxy_pass http://127.0.0.1:{target_port};
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        add_header X-Content-Type-Options "nosniff" always;
    }}
}}"#,
            domain = domain, target_port = target_port
        );

        fs::write(&config_path, content).await.map_err(|e| e.to_string())?;
        if !enabled_link.exists() {
            fs::symlink(&config_path, &enabled_link).await.map_err(|e| e.to_string())?;
        }
        self.test_and_reload().await
    }

    async fn remove_vhost(&self, domain: &str) -> Result<(), String> {
        validate_domain_format(domain)?;

        let config_path = self.base_path.join("sites-available").join(domain);
        let enabled_link = self.base_path.join("sites-enabled").join(domain);
        let _ = fs::remove_file(enabled_link).await;
        let _ = fs::remove_file(config_path).await;
        self.test_and_reload().await
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_validate_domain_format_valid() {
        assert!(validate_domain_format("example.com").is_ok());
        assert!(validate_domain_format("sub.example.com").is_ok());
        assert!(validate_domain_format("my-site.com").is_ok());
        assert!(validate_domain_format("under_score.com").is_ok());
        assert!(validate_domain_format("123.com").is_ok());
    }

    #[test]
    fn test_validate_domain_format_invalid() {
        // Injection attempts
        assert!(validate_domain_format("example.com;").is_err());
        assert!(validate_domain_format("example.com{").is_err());
        assert!(validate_domain_format("example.com}").is_err());
        assert!(validate_domain_format("example.com space").is_err());
        assert!(validate_domain_format("example.com\n").is_err());
        assert!(validate_domain_format("example.com\t").is_err());

        // Path traversal
        assert!(validate_domain_format("../foo").is_err());
        assert!(validate_domain_format("foo/bar").is_err());
        assert!(validate_domain_format("foo\\bar").is_err());

        // Empty
        assert!(validate_domain_format("").is_err());
    }
}
