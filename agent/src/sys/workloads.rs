use super::hosting::{activate_proxy, bound_site, fetch, private_write, run};
use crate::config::AgentConfig;
use crate::server::kari_agent::*;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs;
use std::os::unix::fs::{MetadataExt, PermissionsExt};
use std::path::Path;
use std::process::Stdio;
use std::time::Duration;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::process::Command;

#[derive(Serialize, Deserialize)]
pub(super) struct RuntimeConfig {
    pub runtime: String,
    pub env: HashMap<String, String>,
    pub repository: String,
    pub branch: String,
}

pub(super) fn username(id: &str) -> String {
    format!("ks-{}", &id.replace('-', "")[..24])
}

pub(super) async fn ensure_files(root: &Path, user: &str) -> Result<(), String> {
    fs::create_dir_all(root.join("files")).map_err(|e| e.to_string())?;
    if fs::symlink_metadata(root.join("files"))
        .map_err(|e| e.to_string())?
        .file_type()
        .is_symlink()
    {
        return Err("Site data must not be a link".into());
    }
    run(
        "chown",
        &[
            user,
            root.join("files").to_str().ok_or("Invalid data path")?,
        ],
    )
    .await?;
    fs::set_permissions(root.join("files"), fs::Permissions::from_mode(0o755))
        .map_err(|e| e.to_string())
}

pub(super) async fn output(program: &str, args: &[&str]) -> Result<String, String> {
    let mut child = Command::new(program)
        .args(args)
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::null())
        .kill_on_drop(true)
        .spawn()
        .map_err(|e| e.to_string())?;
    let mut bytes = Vec::new();
    let stdout = child.stdout.take().ok_or("Missing command output")?;
    tokio::time::timeout(
        Duration::from_secs(10),
        stdout.take(65536).read_to_end(&mut bytes),
    )
    .await
    .map_err(|_| "Command output timeout")?
    .map_err(|e| e.to_string())?;
    let status = tokio::time::timeout(Duration::from_secs(10), child.wait())
        .await
        .map_err(|_| "Command timeout")?
        .map_err(|e| e.to_string())?;
    if !status.success() {
        return Err(format!("{program} failed; inspect host logs"));
    }
    String::from_utf8(bytes).map_err(|_| "Command returned invalid UTF-8".into())
}

fn image(value: &str) -> Result<&str, String> {
    let (_, hash) = value
        .rsplit_once("@sha256:")
        .ok_or("A digest-pinned image is required")?;
    if hash.len() != 64 || !hash.bytes().all(|c| c.is_ascii_hexdigit()) {
        return Err("Invalid image digest".into());
    }
    Ok(value)
}

pub(super) async fn network(id: &str) -> Result<String, String> {
    let name = format!("kari-site-{id}");
    let found = output(
        "docker",
        &[
            "network",
            "ls",
            "--filter",
            &format!("name=^{name}$"),
            "--format",
            "{{.Name}}",
        ],
    )
    .await?;
    if found.trim() != name {
        run(
            "docker",
            &["network", "create", "--label", "kari.managed=true", &name],
        )
        .await?;
    }
    Ok(name)
}

async fn npm(
    config: &AgentConfig,
    checkout: &Path,
    uid: u32,
    gid: u32,
    id: &str,
    args: &[&str],
) -> Result<(), String> {
    let name = format!("kari-build-{id}");
    let mount = format!("type=bind,src={},dst=/app", checkout.display());
    let identity = format!("{uid}:{gid}");
    let mut command = vec![
        "run",
        "--name",
        &name,
        "--user",
        &identity,
        "--cap-drop=ALL",
        "--security-opt=no-new-privileges",
        "--memory=512m",
        "--cpus=1",
        "--pids-limit=128",
        "--read-only",
        "--tmpfs",
        "/tmp:rw,nosuid,size=256m",
        "--mount",
        &mount,
        "--workdir",
        "/app",
        "--env",
        "HOME=/tmp",
        image(&config.node_image)?,
        "npm",
    ];
    command.extend_from_slice(args);
    let result = tokio::time::timeout(Duration::from_secs(120), run("docker", &command))
        .await
        .map_err(|_| "Node build exceeded two minutes".to_string())
        .and_then(|v| v);
    // Killing a Docker CLI does not stop its container. Always remove this known
    // build container before returning, including timeout and script failure.
    run("docker", &["rm", "--force", &name]).await?;
    result
}

pub(super) async fn ready(port: &str, domain: &str) -> Result<(), String> {
    let deadline = tokio::time::Instant::now() + Duration::from_secs(20);
    while tokio::time::Instant::now() < deadline {
        let probe = async {
            let mut stream = tokio::net::TcpStream::connect(format!("127.0.0.1:{port}")).await?;
            stream
                .write_all(format!("GET / HTTP/1.0\r\nHost: {domain}\r\n\r\n").as_bytes())
                .await?;
            let mut bytes = [0u8; 64];
            let length = stream.read(&mut bytes).await?;
            let response = String::from_utf8_lossy(&bytes[..length]);
            Ok::<bool, std::io::Error>(
                response.starts_with("HTTP/1.")
                    && response
                        .split_whitespace()
                        .nth(1)
                        .is_some_and(|s| s.starts_with('2') || s.starts_with('3')),
            )
        };
        if matches!(
            tokio::time::timeout(Duration::from_secs(1), probe).await,
            Ok(Ok(true))
        ) {
            return Ok(());
        }
        tokio::time::sleep(Duration::from_millis(200)).await;
    }
    Err("Node application did not serve a successful response at / on PORT=3000".into())
}

pub(super) fn current_container(root: &Path, id: &str) -> Result<String, String> {
    let name = fs::read_to_string(root.join("runtime-container")).map_err(|e| e.to_string())?;
    let prefix = format!("kari-app-{id}-");
    if !name.starts_with(&prefix) || !super::hosting::uuid(&name[prefix.len()..]) {
        return Err("Invalid managed container identity".into());
    }
    Ok(name)
}

pub(super) async fn start_release(
    config: &AgentConfig,
    root: &Path,
    id: &str,
    domain: &str,
    release: &Path,
    release_id: &str,
    settings: &RuntimeConfig,
) -> Result<(), String> {
    let net = network(id).await?;
    let uid = fs::metadata(root.join("files"))
        .map_err(|e| e.to_string())?
        .uid();
    let name = format!("kari-app-{id}-{release_id}");
    let env_path = root.join(format!("env-{release_id}"));
    let mut environment =
        String::from("PORT=3000\nHOST=0.0.0.0\nNODE_ENV=production\nKARI_DATA_DIR=/data\n");
    if settings.env.len() > 64 {
        return Err("At most 64 environment variables are supported".into());
    }
    for (key, value) in &settings.env {
        if key.is_empty()
            || !key
                .bytes()
                .all(|c| c.is_ascii_uppercase() || c.is_ascii_digit() || c == b'_')
            || key.starts_with(|c: char| c.is_ascii_digit())
            || value.contains(['\n', '\r', '\0'])
            || value.len() > 8192
            || matches!(
                key.as_str(),
                "PORT" | "HOST" | "NODE_ENV" | "KARI_DATA_DIR" | "DATABASE_URL"
            )
        {
            return Err("Invalid or reserved environment variable".into());
        }
        environment.push_str(&format!("{key}={value}\n"));
    }
    if let Ok(password) = fs::read_to_string(root.join("database-password")) {
        environment.push_str(&format!(
            "DATABASE_URL=postgres://site:{}@database:5432/site\n",
            password.trim()
        ));
    }
    private_write(&env_path, environment.as_bytes(), 0o600)?;
    let app_mount = format!("type=bind,src={},dst=/app,readonly", release.display());
    let data_mount = format!("type=bind,src={},dst=/data", root.join("files").display());
    let listener = std::net::TcpListener::bind("127.0.0.1:0").map_err(|e| e.to_string())?;
    let binding = format!(
        "127.0.0.1:{}:3000",
        listener.local_addr().map_err(|e| e.to_string())?.port()
    );
    drop(listener);
    run(
        "docker",
        &[
            "run",
            "--detach",
            "--name",
            &name,
            "--label",
            "kari.managed=true",
            "--restart=unless-stopped",
            "--user",
            &format!("{uid}:{uid}"),
            "--cap-drop=ALL",
            "--security-opt=no-new-privileges",
            "--read-only",
            "--memory=512m",
            "--cpus=1",
            "--pids-limit=128",
            "--log-opt",
            "max-size=5m",
            "--log-opt",
            "max-file=2",
            "--tmpfs",
            "/tmp:rw,nosuid,size=64m",
            "--network",
            &net,
            "--publish",
            &binding,
            "--mount",
            &app_mount,
            "--mount",
            &data_mount,
            "--workdir",
            "/app",
            "--env-file",
            env_path.to_str().ok_or("Invalid environment path")?,
            image(&config.node_image)?,
            "npm",
            "start",
        ],
    )
    .await?;
    let activation = async {
        let binding = output("docker", &["port", &name, "3000/tcp"]).await?;
        let port = binding
            .trim()
            .strip_prefix("127.0.0.1:")
            .ok_or("Runtime port is not loopback-only")?;
        port.parse::<u16>().map_err(|_| "Invalid runtime port")?;
        ready(port, domain).await?;
        let previous_port = fs::read(root.join("runtime-port")).ok();
        private_write(&root.join("runtime-port"), port.as_bytes(), 0o600)?;
        let tls = root.join("tls-current");
        if let Err(error) = activate_proxy(
            config,
            id,
            domain,
            if tls.exists() { Some(&tls) } else { None },
        )
        .await
        {
            if let Some(bytes) = previous_port {
                private_write(&root.join("runtime-port"), &bytes, 0o600)?;
            } else {
                fs::remove_file(root.join("runtime-port")).map_err(|e| e.to_string())?;
            }
            return Err(error);
        }
        Ok(())
    }
    .await;
    if let Err(error) = activation {
        run("docker", &["rm", "--force", &name]).await?;
        return Err(error);
    }
    let old = current_container(root, id).ok();
    private_write(&root.join("runtime-container"), name.as_bytes(), 0o600)?;
    private_write(
        &root.join("runtime.json"),
        &serde_json::to_vec(settings).map_err(|e| e.to_string())?,
        0o600,
    )?;
    let next = root.join("next");
    std::os::unix::fs::symlink(release, &next).map_err(|e| e.to_string())?;
    fs::rename(next, root.join("current")).map_err(|e| e.to_string())?;
    if let Some(old) = old {
        run("docker", &["rm", "--force", &old]).await?;
    }
    Ok(())
}

pub async fn deploy_node(config: &AgentConfig, req: StaticSiteRequest) -> Result<String, String> {
    let (root, _staging, checkout, user) = fetch(config, &req).await?;
    ensure_files(&root, &user).await?;
    if !fs::symlink_metadata(checkout.join("package-lock.json"))
        .map_err(|_| "Node sites require package-lock.json")?
        .is_file()
    {
        return Err("Lockfile must be a regular file".into());
    }
    let metadata = fs::metadata(&checkout).map_err(|e| e.to_string())?;
    npm(
        config,
        &checkout,
        metadata.uid(),
        metadata.gid(),
        &req.deployment_id,
        &["ci", "--ignore-scripts", "--no-audit", "--no-fund"],
    )
    .await?;
    npm(
        config,
        &checkout,
        metadata.uid(),
        metadata.gid(),
        &req.deployment_id,
        &["run", "build", "--if-present"],
    )
    .await?;
    let release = root.join("releases").join(&req.deployment_id);
    fs::rename(checkout, &release).map_err(|e| e.to_string())?;
    run(
        "chown",
        &[
            "-hRP",
            "root:root",
            release.to_str().ok_or("Invalid release path")?,
        ],
    )
    .await?;
    // Repository links stay inside a container mount; root never recursively
    // follows them. Published source is mounted read-only into the runtime.
    let environment = fs::read(root.join("runtime-env.json"))
        .ok()
        .map(|bytes| serde_json::from_slice(&bytes).map_err(|e| e.to_string()))
        .transpose()?
        .unwrap_or_default();
    let settings = RuntimeConfig {
        runtime: "nodejs".into(),
        env: environment,
        repository: req.repo_url,
        branch: req.branch,
    };
    start_release(
        config,
        &root,
        &req.site_id,
        &req.domain_name,
        &release,
        &req.deployment_id,
        &settings,
    )
    .await?;
    Ok("Node application built, served a successful health response, and activated".into())
}

async fn application_logs(name: &str) -> Result<String, String> {
    let mut child = Command::new("docker")
        .args(["logs", "--tail", "100", name])
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .kill_on_drop(true)
        .spawn()
        .map_err(|e| e.to_string())?;
    let stdout = child.stdout.take().ok_or("Missing stdout")?;
    let stderr = child.stderr.take().ok_or("Missing stderr")?;
    let reads = async {
        tokio::try_join!(
            async {
                let mut bytes = Vec::new();
                stdout.take(32769).read_to_end(&mut bytes).await?;
                Ok::<_, std::io::Error>(bytes)
            },
            async {
                let mut bytes = Vec::new();
                stderr.take(32769).read_to_end(&mut bytes).await?;
                Ok::<_, std::io::Error>(bytes)
            }
        )
    };
    let (mut out, mut err) = tokio::time::timeout(Duration::from_secs(10), reads)
        .await
        .map_err(|_| "Log read timeout")?
        .map_err(|e| e.to_string())?;
    let truncated = out.len() > 32768 || err.len() > 32768;
    if truncated && child.try_wait().map_err(|e| e.to_string())?.is_none() {
        child.kill().await.map_err(|e| e.to_string())?;
    }
    let status = child.wait().await.map_err(|e| e.to_string())?;
    if !truncated && !status.success() {
        return Err("Application logs unavailable".into());
    }
    out.truncate(32768);
    err.truncate(32768);
    Ok(format!(
        "{}{}{}",
        String::from_utf8_lossy(&out),
        String::from_utf8_lossy(&err),
        if truncated {
            "\n[Log output truncated]"
        } else {
            ""
        }
    ))
}

pub async fn runtime(config: &AgentConfig, req: RuntimeRequest) -> Result<RuntimeStatus, String> {
    if req.action == "initialize" {
        super::hosting::prepare(config, &req.site_id, &req.domain_name).await?;
        return Ok(RuntimeStatus::default());
    }
    let root = bound_site(config, &req.site_id, &req.domain_name)?;
    if req.action == "environment" {
        if req.env_vars.len() > 64
            || req.env_vars.iter().any(|(key, value)| {
                key.is_empty()
                    || key.starts_with(|c: char| c.is_ascii_digit())
                    || !key
                        .bytes()
                        .all(|c| c.is_ascii_uppercase() || c.is_ascii_digit() || c == b'_')
                    || value.contains(['\n', '\r', '\0'])
                    || value.len() > 8192
                    || matches!(
                        key.as_str(),
                        "PORT" | "HOST" | "NODE_ENV" | "KARI_DATA_DIR" | "DATABASE_URL"
                    )
            })
        {
            return Err("Invalid or reserved environment variable".into());
        }
        private_write(
            &root.join("runtime-env.json"),
            &serde_json::to_vec(&req.env_vars).map_err(|e| e.to_string())?,
            0o600,
        )?;
        return Ok(RuntimeStatus::default());
    }
    if req.action == "status" && !root.join("runtime-container").exists() {
        return Ok(RuntimeStatus::default());
    }
    let name = current_container(&root, &req.site_id)?;
    match req.action.as_str() {
        "status" => {}
        "logs" => {
            let running = output(
                "docker",
                &["inspect", "--format", "{{.State.Running}}", &name],
            )
            .await?
            .trim()
                == "true";
            return Ok(RuntimeStatus {
                running,
                logs: application_logs(&name).await?,
            });
        }
        "start" | "restart" => {
            run("docker", &[&req.action, &name]).await?;
            let port = output("docker", &["port", &name, "3000/tcp"]).await?;
            ready(
                port.trim()
                    .strip_prefix("127.0.0.1:")
                    .ok_or("Invalid runtime binding")?,
                &req.domain_name,
            )
            .await?;
        }
        "stop" => {
            run("docker", &["stop", "--time", "10", &name]).await?;
        }
        _ => return Err("Unsupported runtime action".into()),
    }
    let running = output(
        "docker",
        &["inspect", "--format", "{{.State.Running}}", &name],
    )
    .await?
    .trim()
        == "true";
    Ok(RuntimeStatus {
        running,
        logs: String::new(),
    })
}

pub async fn database(
    config: &AgentConfig,
    mut req: DatabaseRequest,
) -> Result<DatabaseStatus, String> {
    use zeroize::Zeroize;
    let root = bound_site(config, &req.site_id, &req.domain_name)?;
    let name = format!("kari-db-{}", req.site_id);
    let exists = !output(
        "docker",
        &[
            "ps",
            "--all",
            "--filter",
            &format!("name=^{name}$"),
            "--format",
            "{{.Names}}",
        ],
    )
    .await?
    .trim()
    .is_empty();
    if req.action == "status" {
        return Ok(DatabaseStatus {
            exists,
            host: "database:5432".into(),
            database: "site".into(),
            username: "site".into(),
        });
    }
    if req.action == "delete" {
        if exists {
            run("docker", &["rm", "--force", &name]).await?;
        }
        if root.join("database").exists() {
            fs::remove_dir_all(root.join("database")).map_err(|e| e.to_string())?;
        }
        if root.join("database-password").exists() {
            fs::remove_file(root.join("database-password")).map_err(|e| e.to_string())?;
        }
        return Ok(DatabaseStatus::default());
    }
    if req.action != "create"
        || exists
        || req.password.len() != 64
        || !req.password.bytes().all(|c| c.is_ascii_hexdigit())
    {
        return Err("Database exists or request is invalid".into());
    }
    let net = network(&req.site_id).await?;
    fs::create_dir(root.join("database")).map_err(|e| e.to_string())?;
    let mut random = [0u8; 32];
    std::io::Read::read_exact(
        &mut fs::File::open("/dev/urandom").map_err(|e| e.to_string())?,
        &mut random,
    )
    .map_err(|e| e.to_string())?;
    let admin: String = random.iter().map(|b| format!("{b:02x}")).collect();
    let env = root.join("database-init.env");
    private_write(
        &env,
        format!("POSTGRES_PASSWORD={admin}\n").as_bytes(),
        0o600,
    )?;
    let mount = format!(
        "type=bind,src={},dst=/var/lib/postgresql/data",
        root.join("database").display()
    );
    run(
        "docker",
        &[
            "run",
            "--detach",
            "--name",
            &name,
            "--label",
            "kari.managed=true",
            "--restart=unless-stopped",
            "--memory=512m",
            "--pids-limit=128",
            "--log-opt",
            "max-size=5m",
            "--log-opt",
            "max-file=2",
            "--network",
            &net,
            "--network-alias",
            "database",
            "--env-file",
            env.to_str().ok_or("Invalid database configuration")?,
            "--mount",
            &mount,
            image(&config.postgres_image)?,
        ],
    )
    .await?;
    let mut ready = false;
    for _ in 0..60 {
        if output(
            "docker",
            &[
                "exec",
                &name,
                "pg_isready",
                "-h",
                "127.0.0.1",
                "-U",
                "postgres",
            ],
        )
        .await
        .is_ok()
        {
            ready = true;
            break;
        }
        tokio::time::sleep(Duration::from_millis(500)).await;
    }
    if !ready {
        return Err("Customer database did not become ready; inspect its container".into());
    }
    let statement = format!(
        "CREATE ROLE site LOGIN PASSWORD '{}' NOSUPERUSER NOCREATEDB NOCREATEROLE; CREATE DATABASE site OWNER site; REVOKE CONNECT ON DATABASE postgres FROM PUBLIC;",
        req.password
    );
    sql(&name, &statement).await?;
    private_write(
        &root.join("database-password"),
        req.password.as_bytes(),
        0o600,
    )?;
    req.password.zeroize();
    Ok(DatabaseStatus {
        exists: true,
        host: "database:5432".into(),
        database: "site".into(),
        username: "site".into(),
    })
}

pub(super) async fn sql(container: &str, statement: &str) -> Result<(), String> {
    let mut child = Command::new("docker")
        .args([
            "exec",
            "-i",
            container,
            "psql",
            "-U",
            "postgres",
            "-d",
            "postgres",
            "-v",
            "ON_ERROR_STOP=1",
        ])
        .stdin(Stdio::piped())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .kill_on_drop(true)
        .spawn()
        .map_err(|e| e.to_string())?;
    child
        .stdin
        .take()
        .ok_or("Missing SQL input")?
        .write_all(statement.as_bytes())
        .await
        .map_err(|e| e.to_string())?;
    if !child.wait().await.map_err(|e| e.to_string())?.success() {
        return Err("Customer database operation failed".into());
    }
    Ok(())
}

pub async fn sftp(config: &AgentConfig, req: SftpRequest) -> Result<SftpStatus, String> {
    let root = bound_site(config, &req.site_id, &req.domain_name)?;
    let user = username(&req.site_id);
    ensure_files(&root, &user).await?;
    // These are public keys, readable by sshd under the target UID. Root owns
    // the directory and files; the site cannot add authorization options.
    let keys = Path::new("/etc/ssh/kari_keys");
    fs::create_dir_all(keys).map_err(|e| e.to_string())?;
    fs::set_permissions(keys, fs::Permissions::from_mode(0o755)).map_err(|e| e.to_string())?;
    let key_path = keys.join(&user);
    let configuration =
        Path::new("/etc/ssh/sshd_config.d").join(format!("kari-{}.conf", req.site_id));
    if req.action == "status" {
        return Ok(SftpStatus {
            enabled: key_path.exists(),
            username: user,
        });
    }
    if req.action == "disable" {
        if key_path.exists() {
            fs::remove_file(&key_path).map_err(|e| e.to_string())?;
        }
        let closed = Command::new("pkill")
            .args(["-KILL", "-u", &user, "-x", "sshd"])
            .status()
            .await
            .map_err(|e| e.to_string())?;
        if !closed.success() && closed.code() != Some(1) {
            return Err("Could not close the site's SFTP sessions".into());
        }
        return Ok(SftpStatus {
            enabled: false,
            username: user,
        });
    }
    if req.action != "enable"
        || req.public_key.len() > 16384
        || req.public_key.contains(['\n', '\r'])
        || !(req.public_key.starts_with("ssh-ed25519 ") || req.public_key.starts_with("ssh-rsa "))
    {
        return Err("Supply one OpenSSH Ed25519 or RSA public key without options".into());
    }
    let candidate = tempfile::NamedTempFile::new_in(keys).map_err(|e| e.to_string())?;
    fs::write(candidate.path(), &req.public_key).map_err(|e| e.to_string())?;
    run(
        "ssh-keygen",
        &[
            "-l",
            "-f",
            candidate.path().to_str().ok_or("Invalid key path")?,
        ],
    )
    .await?;
    let content = format!(
        "Match User {user}\n ChrootDirectory {}\n ForceCommand internal-sftp -d /files\n AuthorizedKeysFile {}\n AuthenticationMethods publickey\n PasswordAuthentication no\n DisableForwarding yes\n PermitTTY no\n X11Forwarding no\nMatch all\n",
        root.display(),
        key_path.display()
    );
    let previous = fs::read(&configuration).ok();
    private_write(&configuration, content.as_bytes(), 0o600)?;
    if let Err(error) = run("/usr/sbin/sshd", &["-t"]).await {
        if let Some(bytes) = previous {
            private_write(&configuration, &bytes, 0o600)?;
        } else {
            fs::remove_file(&configuration).map_err(|e| e.to_string())?;
        }
        return Err(error);
    }
    run("systemctl", &["reload", "ssh"]).await?;
    private_write(&key_path, req.public_key.as_bytes(), 0o644)?;
    Ok(SftpStatus {
        enabled: true,
        username: user,
    })
}

pub async fn host_services(req: HostServiceRequest) -> Result<HostServiceStatus, String> {
    if req.action == "reload" && req.service == "nginx" {
        run("nginx", &["-t"]).await?;
        run("systemctl", &["reload", "nginx"]).await?;
    } else if req.action != "status" {
        return Err("Only Nginx reload and service status are supported".into());
    }
    let mut services = HashMap::new();
    for service in ["nginx", "docker", "ssh", "kari-agent"] {
        let state = Command::new("systemctl")
            .args(["is-active", service])
            .output()
            .await
            .map_err(|e| e.to_string())?;
        services.insert(
            service.to_string(),
            String::from_utf8_lossy(&state.stdout).trim().to_string(),
        );
    }
    Ok(HostServiceStatus { services })
}
