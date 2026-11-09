use crate::config::AgentConfig;
use crate::server::kari_agent::{SiteCertificate, SiteChallenge, StaticSiteRequest};
use std::fs;
use std::os::unix::fs::{PermissionsExt, symlink};
use std::path::{Path, PathBuf};
use std::process::Stdio;
use std::time::Duration;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::process::Command;
use zeroize::Zeroize;

pub(super) fn uuid(value: &str) -> bool {
    value.len() == 36
        && value.bytes().enumerate().all(|(i, c)| {
            if [8, 13, 18, 23].contains(&i) {
                c == b'-'
            } else {
                c.is_ascii_hexdigit() && !c.is_ascii_uppercase()
            }
        })
}

pub fn validate_domain(domain: &str) -> Result<(), String> {
    if domain.len() > 253
        || !domain.contains('.')
        || !domain.split('.').all(|label| {
            !label.is_empty()
                && label.len() <= 63
                && !label.starts_with('-')
                && !label.ends_with('-')
                && label
                    .bytes()
                    .all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == b'-')
        })
    {
        return Err("A lowercase DNS hostname is required".into());
    }
    Ok(())
}

fn site_path(config: &AgentConfig, id: &str, domain: &str) -> Result<PathBuf, String> {
    validate_domain(domain)?;
    if !uuid(id) {
        return Err("Invalid site UUID".into());
    }
    for path in [
        &config.web_root,
        &config.proxy_conf_dir,
        &config.ssl_storage_dir,
    ] {
        if !path.is_absolute()
            || !path
                .to_string_lossy()
                .bytes()
                .all(|c| c.is_ascii_alphanumeric() || b"/-_.".contains(&c))
        {
            return Err(
                "Hosting requires absolute configuration paths without special characters".into(),
            );
        }
        let mut current = PathBuf::new();
        for component in path.components() {
            current.push(component);
            if let Ok(meta) = fs::symlink_metadata(&current)
                && (meta.file_type().is_symlink() || !meta.is_dir())
            {
                return Err("Hosting path contains a link or non-directory".into());
            }
        }
    }
    Ok(config.web_root.join("sites").join(id))
}

pub(super) fn bound_site(config: &AgentConfig, id: &str, domain: &str) -> Result<PathBuf, String> {
    let path = site_path(config, id, domain)?;
    if fs::read_to_string(path.join("domain")).map_err(|e| e.to_string())? != domain {
        return Err("Site/domain binding does not match".into());
    }
    Ok(path)
}

pub(super) async fn run(program: &str, args: &[&str]) -> Result<(), String> {
    let status = Command::new(program)
        .args(args)
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .kill_on_drop(true)
        .status()
        .await
        .map_err(|e| e.to_string())?;
    if !status.success() {
        return Err(format!("{program} failed; inspect the host journal"));
    }
    Ok(())
}

pub(super) fn private_write(path: &Path, bytes: &[u8], mode: u32) -> Result<(), String> {
    use std::io::Write;
    let parent = path.parent().ok_or("Missing parent")?;
    let mut temp = tempfile::NamedTempFile::new_in(parent).map_err(|e| e.to_string())?;
    temp.as_file()
        .set_permissions(fs::Permissions::from_mode(mode))
        .map_err(|e| e.to_string())?;
    temp.write_all(bytes).map_err(|e| e.to_string())?;
    temp.as_file().sync_all().map_err(|e| e.to_string())?;
    temp.persist(path).map_err(|e| e.to_string())?;
    Ok(())
}

fn copy_public(
    source: &Path,
    destination: &Path,
    count: &mut usize,
    size: &mut u64,
) -> Result<(), String> {
    fs::create_dir(destination).map_err(|e| e.to_string())?;
    fs::set_permissions(destination, fs::Permissions::from_mode(0o755))
        .map_err(|e| e.to_string())?;
    for entry in fs::read_dir(source).map_err(|e| e.to_string())? {
        let entry = entry.map_err(|e| e.to_string())?;
        let name = entry.file_name();
        if name.to_string_lossy().starts_with('.') {
            continue;
        }
        *count += 1;
        let meta = fs::symlink_metadata(entry.path()).map_err(|e| e.to_string())?;
        *size += meta.len();
        if *count > 10_000 || *size > 100 * 1024 * 1024 {
            return Err("Site exceeds 10,000 entries or 100 MiB".into());
        }
        let target = destination.join(&name);
        if meta.is_dir() {
            copy_public(&entry.path(), &target, count, size)?;
        } else if meta.is_file() {
            fs::copy(entry.path(), &target).map_err(|e| e.to_string())?;
            fs::set_permissions(target, fs::Permissions::from_mode(0o644))
                .map_err(|e| e.to_string())?;
        } else {
            return Err("Site may contain only ordinary files and directories".into());
        }
    }
    Ok(())
}

pub(super) async fn activate_proxy(
    config: &AgentConfig,
    id: &str,
    domain: &str,
    tls: Option<&Path>,
) -> Result<(), String> {
    let root = bound_site(config, id, domain)?;
    let challenge = root.join("challenge");
    let mut content = format!(
        "server {{\n listen 80;\n server_name {domain};\n location ^~ /.well-known/acme-challenge/ {{ alias {}/; }}\n",
        challenge.display()
    );
    let mut static_location = format!(
        "root {}/current;\n disable_symlinks if_not_owner;\n index index.html;\n location ~ /\\. {{ deny all; }}\n location / {{ try_files $uri $uri/ =404; }}\n",
        root.display()
    );
    let mut revision = id.to_string();
    if let Ok(port) = fs::read_to_string(root.join("runtime-port")) {
        let port: u16 = port.trim().parse().map_err(|_| "Invalid runtime port")?;
        revision = port.to_string();
        static_location = format!(
            "location / {{ proxy_pass http://127.0.0.1:{port}; proxy_set_header Host $host; proxy_set_header X-Forwarded-Proto $scheme; }}\n"
        );
    } else {
        static_location.push_str(&format!(
            "location /files/ {{ alias {}/files/; }}\n",
            root.display()
        ));
    }
    // A successful reload command only signals the master. This loopback-only
    // location proves a new worker has accepted this exact routing revision.
    content.push_str(&format!("location = /.__kari_route {{ if ($remote_addr != 127.0.0.1) {{ return 404; }} return 200 \"{revision}\"; }}\n"));
    if let Some(cert) = tls {
        content.push_str(&format!("location / {{ return 301 https://{domain}$request_uri; }}\n}}\nserver {{\n listen 443 ssl;\n server_name {domain};\n ssl_certificate {}/fullchain.pem;\n ssl_certificate_key {}/privkey.pem;\n ssl_protocols TLSv1.2 TLSv1.3;\n{static_location}}}\n", cert.display(), cert.display()));
    } else {
        content.push_str(&format!("{static_location}}}\n"));
    }
    let available = config.proxy_conf_dir.join("sites-available");
    let enabled = config.proxy_conf_dir.join("sites-enabled");
    let name = format!("kari-site-{id}.conf");
    let target = available.join(&name);
    let link = enabled.join(&name);
    let previous = fs::read(&target).ok();
    if let Ok(meta) = fs::symlink_metadata(&target)
        && !meta.is_file()
    {
        return Err("Unsafe proxy target".into());
    }
    if let Ok(existing) = fs::read_link(&link)
        && existing != target
    {
        return Err("Proxy link is not managed by this site".into());
    }
    private_write(&target, content.as_bytes(), 0o644)?;
    let new_link = fs::symlink_metadata(&link).is_err();
    if new_link {
        symlink(&target, &link).map_err(|e| e.to_string())?;
    }
    let result = async {
        run("nginx", &["-t"]).await?;
        run("systemctl", &["reload", "nginx"]).await?;
        wait_route(domain, &revision).await?;
        if let Some(certificate) = tls {
            wait_certificate(domain, certificate).await?;
        }
        Ok(())
    }
    .await;
    if result.is_err() {
        if let Some(bytes) = previous {
            private_write(&target, &bytes, 0o644)?;
        } else {
            fs::remove_file(&target).map_err(|e| e.to_string())?;
        }
        if new_link {
            fs::remove_file(&link).map_err(|e| e.to_string())?;
        }
        run("nginx", &["-t"]).await?;
        run("systemctl", &["reload", "nginx"]).await?;
    }
    result
}

async fn wait_route(domain: &str, revision: &str) -> Result<(), String> {
    let deadline = tokio::time::Instant::now() + Duration::from_secs(10);
    while tokio::time::Instant::now() < deadline {
        let probe = async {
            let mut stream = tokio::net::TcpStream::connect("127.0.0.1:80").await?;
            stream
                .write_all(
                    format!("GET /.__kari_route HTTP/1.0\r\nHost: {domain}\r\n\r\n").as_bytes(),
                )
                .await?;
            let mut response = Vec::new();
            stream.take(2048).read_to_end(&mut response).await?;
            let text = String::from_utf8_lossy(&response);
            Ok::<bool, std::io::Error>(
                text.starts_with("HTTP/1.1 200")
                    && text
                        .split_once("\r\n\r\n")
                        .is_some_and(|(_, body)| body == revision),
            )
        };
        if matches!(
            tokio::time::timeout(Duration::from_secs(1), probe).await,
            Ok(Ok(true))
        ) {
            return Ok(());
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
    Err("Nginx did not begin serving the expected route; activation reverted".into())
}

async fn wait_certificate(domain: &str, directory: &Path) -> Result<(), String> {
    fn leaf(pem: &[u8]) -> Option<String> {
        let text = std::str::from_utf8(pem).ok()?;
        let first = text.split("-----BEGIN CERTIFICATE-----").nth(1)?;
        Some(
            first
                .split("-----END CERTIFICATE-----")
                .next()?
                .split_whitespace()
                .collect(),
        )
    }
    let expected = leaf(&fs::read(directory.join("fullchain.pem")).map_err(|e| e.to_string())?)
        .ok_or("Invalid certificate PEM")?;
    let deadline = tokio::time::Instant::now() + Duration::from_secs(10);
    while tokio::time::Instant::now() < deadline {
        let output = tokio::time::timeout(
            Duration::from_secs(2),
            Command::new("openssl")
                .args([
                    "s_client",
                    "-connect",
                    "127.0.0.1:443",
                    "-servername",
                    domain,
                    "-showcerts",
                ])
                .stdin(Stdio::null())
                .stderr(Stdio::null())
                .kill_on_drop(true)
                .output(),
        )
        .await;
        if let Ok(Ok(output)) = output
            && output.status.success()
            && leaf(&output.stdout).as_ref() == Some(&expected)
        {
            return Ok(());
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
    Err("Nginx did not begin serving the expected certificate; activation reverted".into())
}

pub(super) async fn prepare(
    config: &AgentConfig,
    id: &str,
    domain: &str,
) -> Result<(PathBuf, String), String> {
    let root = site_path(config, id, domain)?;
    fs::create_dir_all(root.join("releases")).map_err(|e| e.to_string())?;
    for directory in [
        &config.web_root,
        &config.web_root.join("sites"),
        &root,
        &root.join("releases"),
    ] {
        fs::set_permissions(directory, fs::Permissions::from_mode(0o755))
            .map_err(|e| e.to_string())?;
    }
    if root.join("domain").exists() {
        bound_site(config, id, domain)?;
    } else {
        private_write(&root.join("domain"), domain.as_bytes(), 0o600)?;
    }
    fs::create_dir_all(root.join("challenge")).map_err(|e| e.to_string())?;
    fs::set_permissions(root.join("challenge"), fs::Permissions::from_mode(0o755))
        .map_err(|e| e.to_string())?;
    let user = format!("ks-{}", id.replace('-', ""));
    let user = user[..27].to_string();
    let exists = Command::new("id")
        .args(["-u", &user])
        .output()
        .await
        .map_err(|e| e.to_string())?;
    if !exists.status.success() {
        run(
            "useradd",
            &[
                "--system",
                "--user-group",
                "--no-create-home",
                "--shell",
                "/usr/sbin/nologin",
                &user,
            ],
        )
        .await?;
    } else if String::from_utf8_lossy(&exists.stdout).trim() == "0" {
        return Err("Refusing root fetch identity".into());
    }
    super::workloads::ensure_files(&root, &user).await?;
    Ok((root, user))
}

pub(super) async fn fetch(
    config: &AgentConfig,
    req: &StaticSiteRequest,
) -> Result<(PathBuf, tempfile::TempDir, PathBuf, String), String> {
    if !uuid(&req.deployment_id)
        || !req.repo_url.starts_with("https://")
        || req.repo_url.len() > 2048
        || req.repo_url[8..].contains(['@', '#', '?', '\\'])
        || req
            .repo_url
            .bytes()
            .any(|c| c.is_ascii_control() || c == b' ')
        || req.branch.is_empty()
        || req.branch.starts_with('-')
        || req.branch.len() > 100
        || !req
            .branch
            .bytes()
            .all(|c| c.is_ascii_alphanumeric() || b"/._-".contains(&c))
    {
        return Err("A public HTTPS Git URL, branch, and deployment UUID are required".into());
    }
    let (root, user) = prepare(config, &req.site_id, &req.domain_name).await?;
    let staging = tempfile::Builder::new()
        .prefix("kari-fetch-")
        .tempdir_in(&root)
        .map_err(|e| e.to_string())?;
    run(
        "chown",
        &[
            &user,
            staging.path().to_str().ok_or("Invalid staging path")?,
        ],
    )
    .await?;
    // Git reads untrusted repository data only inside a bounded, per-site service.
    let checkout = staging.path().join("checkout");
    let unit = format!("kari-fetch-{}", req.deployment_id);
    let write_path = format!("ReadWritePaths={}", staging.path().display());
    let user_property = format!("User={user}");
    run(
        "systemd-run",
        &[
            "--wait",
            "--collect",
            "--unit",
            &unit,
            "--property",
            &user_property,
            "--property",
            "ProtectSystem=strict",
            "--property",
            "ProtectHome=yes",
            "--property",
            "PrivateTmp=yes",
            "--property",
            "PrivateDevices=yes",
            "--property",
            "NoNewPrivileges=yes",
            "--property",
            "CapabilityBoundingSet=",
            "--property",
            "RestrictSUIDSGID=yes",
            "--property",
            "TasksMax=32",
            "--property",
            "MemoryMax=256M",
            "--property",
            "RuntimeMaxSec=90",
            "--property",
            &write_path,
            "/usr/bin/env",
            "-i",
            "PATH=/usr/bin:/bin",
            "HOME=/nonexistent",
            "GIT_TERMINAL_PROMPT=0",
            "/usr/bin/git",
            "-c",
            "core.hooksPath=/dev/null",
            "-c",
            "protocol.file.allow=never",
            "-c",
            "protocol.ext.allow=never",
            "clone",
            "--depth=1",
            "--single-branch",
            "--branch",
            &req.branch,
            "--",
            &req.repo_url,
            checkout.to_str().ok_or("Invalid checkout path")?,
        ],
    )
    .await?;
    Ok((root, staging, checkout, user))
}

pub async fn deploy(config: &AgentConfig, req: StaticSiteRequest) -> Result<String, String> {
    let (root, _staging, checkout, _user) = fetch(config, &req).await?;
    if !fs::symlink_metadata(checkout.join("index.html"))
        .map_err(|_| "Repository root must contain index.html")?
        .is_file()
    {
        return Err("index.html must be an ordinary file".into());
    }
    let release = root.join("releases").join(&req.deployment_id);
    let result = copy_public(&checkout, &release, &mut 0, &mut 0);
    if result.is_err() {
        fs::remove_dir_all(&release).map_err(|e| e.to_string())?;
    }
    result?;
    let current = root.join("current");
    let candidate = root.join("next");
    symlink(&release, &candidate).map_err(|e| e.to_string())?;
    let tls_dir = root.join("tls-current");
    let tls = if tls_dir.exists() {
        Some(tls_dir.as_path())
    } else {
        None
    };
    if let Err(error) = activate_proxy(config, &req.site_id, &req.domain_name, tls).await {
        fs::remove_file(&candidate).map_err(|e| e.to_string())?;
        return Err(error);
    }
    fs::rename(&candidate, &current).map_err(|e| e.to_string())?;
    let source = super::workloads::RuntimeConfig {
        runtime: "static".into(),
        env: std::collections::HashMap::new(),
        repository: req.repo_url,
        branch: req.branch,
    };
    private_write(
        &root.join("source.json"),
        &serde_json::to_vec(&source).map_err(|e| e.to_string())?,
        0o600,
    )?;
    Ok("Static site published; no repository code or build command was executed".into())
}

pub fn challenge(config: &AgentConfig, req: SiteChallenge) -> Result<(), String> {
    let root = bound_site(config, &req.site_id, &req.domain_name)?;
    if req.token.is_empty()
        || req.token.len() > 256
        || !req
            .token
            .bytes()
            .all(|c| c.is_ascii_alphanumeric() || b"-_".contains(&c))
        || req.key_authorization.len() > 512
    {
        return Err("Invalid ACME token".into());
    }
    let path = root.join("challenge").join(req.token);
    if req.remove {
        match fs::remove_file(path) {
            Ok(()) => Ok(()),
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(()),
            Err(e) => Err(e.to_string()),
        }
    } else {
        private_write(&path, req.key_authorization.as_bytes(), 0o644)
    }
}

pub async fn certificate(config: &AgentConfig, mut req: SiteCertificate) -> Result<(), String> {
    let root = bound_site(config, &req.site_id, &req.domain_name)?;
    if req.fullchain_pem.len() > 65536 || req.privkey_pem.len() > 16384 {
        return Err("Certificate payload too large".into());
    }
    let dir = tempfile::Builder::new()
        .prefix("tls-")
        .tempdir_in(&root)
        .map_err(|e| e.to_string())?;
    let result = private_write(&dir.path().join("privkey.pem"), &req.privkey_pem, 0o600);
    req.privkey_pem.zeroize();
    result?;
    private_write(&dir.path().join("fullchain.pem"), &req.fullchain_pem, 0o644)?;
    run(
        "openssl",
        &[
            "x509",
            "-in",
            dir.path()
                .join("fullchain.pem")
                .to_str()
                .ok_or("Invalid certificate path")?,
            "-noout",
            "-checkhost",
            &req.domain_name,
            "-checkend",
            "0",
        ],
    )
    .await?;
    // Nginx verifies the key/certificate pair before activation; failed candidates
    // leave the previous configuration and key files intact.
    activate_proxy(config, &req.site_id, &req.domain_name, Some(dir.path())).await?;
    let permanent = dir.keep();
    let next = root.join("tls-next");
    symlink(&permanent, &next).map_err(|e| e.to_string())?;
    fs::rename(next, root.join("tls-current")).map_err(|e| e.to_string())?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn published_files_reject_links_and_hidden_secrets() {
        let dir = tempfile::tempdir().unwrap();
        let source = dir.path().join("source");
        fs::create_dir(&source).unwrap();
        fs::write(source.join("index.html"), "hello").unwrap();
        fs::write(source.join(".env"), "secret").unwrap();
        let destination = dir.path().join("public");
        copy_public(&source, &destination, &mut 0, &mut 0).unwrap();
        assert!(!destination.join(".env").exists());
        symlink("/etc/passwd", source.join("leak")).unwrap();
        assert!(copy_public(&source, &dir.path().join("bad"), &mut 0, &mut 0).is_err());
        assert!(validate_domain("example.com;include").is_err());
    }
}
