use super::hosting::{activate_proxy, bound_site, private_write, run, uuid};
use super::workloads::{self, RuntimeConfig, current_container, output};
use crate::config::AgentConfig;
use crate::server::kari_agent::*;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::{Component, Path};
use std::process::Stdio;
use tokio::process::Command;

#[derive(Serialize, Deserialize)]
struct Snapshot {
    created_at: String,
    hashes: HashMap<String, String>,
    runtime: Option<RuntimeConfig>,
    pending_env: Option<HashMap<String, String>>,
    database: bool,
    certificate: bool,
}

fn copy_tree(
    source: &Path,
    destination: &Path,
    base: &Path,
    entries: &mut usize,
    size: &mut u64,
    depth: usize,
    links: bool,
) -> Result<(), String> {
    if depth > 64 {
        return Err("Backup tree is too deep".into());
    }
    fs::create_dir(destination).map_err(|e| e.to_string())?;
    fs::set_permissions(destination, fs::Permissions::from_mode(0o755))
        .map_err(|e| e.to_string())?;
    for entry in fs::read_dir(source).map_err(|e| e.to_string())? {
        let entry = entry.map_err(|e| e.to_string())?;
        let name = entry.file_name();
        if name == ".git" {
            continue;
        }
        let metadata = fs::symlink_metadata(entry.path()).map_err(|e| e.to_string())?;
        *entries += 1;
        *size += metadata.len();
        if *entries > 100_000 || *size > 512 * 1024 * 1024 {
            return Err("Backup exceeds 100,000 entries or 512 MiB of files".into());
        }
        let target = destination.join(name);
        if metadata.is_dir() {
            copy_tree(
                &entry.path(),
                &target,
                base,
                entries,
                size,
                depth + 1,
                links,
            )?;
        } else if metadata.is_file() {
            fs::copy(entry.path(), &target).map_err(|e| e.to_string())?;
            fs::set_permissions(
                &target,
                fs::Permissions::from_mode(if metadata.permissions().mode() & 0o111 != 0 {
                    0o755
                } else {
                    0o644
                }),
            )
            .map_err(|e| e.to_string())?;
        } else if metadata.file_type().is_symlink() && links {
            let link = fs::read_link(entry.path()).map_err(|e| e.to_string())?;
            let entry_path = entry.path();
            let relative = entry_path
                .parent()
                .ok_or("Missing link parent")?
                .strip_prefix(base)
                .map_err(|e| e.to_string())?;
            let mut level = relative.components().count();
            for component in link.components() {
                match component {
                    Component::Normal(_) => level += 1,
                    Component::CurDir => {}
                    Component::ParentDir if level > 0 => level -= 1,
                    _ => return Err("Backup link escapes its release".into()),
                }
            }
            std::os::unix::fs::symlink(link, target).map_err(|e| e.to_string())?;
        } else {
            return Err("Backup refuses links in writable data and special files".into());
        }
    }
    Ok(())
}

fn hashes(root: &Path, folder: &Path, result: &mut HashMap<String, String>) -> Result<(), String> {
    for entry in fs::read_dir(folder).map_err(|e| e.to_string())? {
        let entry = entry.map_err(|e| e.to_string())?;
        let path = entry.path();
        let meta = fs::symlink_metadata(&path).map_err(|e| e.to_string())?;
        if meta.is_dir() {
            hashes(root, &path, result)?;
        } else {
            let value = if meta.file_type().is_symlink() {
                format!(
                    "link:{}",
                    fs::read_link(&path).map_err(|e| e.to_string())?.display()
                )
            } else {
                let out = std::process::Command::new("sha256sum")
                    .arg(&path)
                    .output()
                    .map_err(|e| e.to_string())?;
                if !out.status.success() {
                    return Err("Backup checksum failed".into());
                }
                String::from_utf8(out.stdout)
                    .map_err(|e| e.to_string())?
                    .split_whitespace()
                    .next()
                    .ok_or("Missing checksum")?
                    .to_string()
            };
            result.insert(
                path.strip_prefix(root)
                    .map_err(|e| e.to_string())?
                    .to_str()
                    .ok_or("Backup filenames must be UTF-8")?
                    .to_string(),
                value,
            );
        }
    }
    Ok(())
}

fn fresh_id() -> Result<String, String> {
    let mut bytes = [0u8; 16];
    std::io::Read::read_exact(
        &mut fs::File::open("/dev/urandom").map_err(|e| e.to_string())?,
        &mut bytes,
    )
    .map_err(|e| e.to_string())?;
    let hex: String = bytes.iter().map(|b| format!("{b:02x}")).collect();
    Ok(format!(
        "{}-{}-{}-{}-{}",
        &hex[..8],
        &hex[8..12],
        &hex[12..16],
        &hex[16..20],
        &hex[20..]
    ))
}

async fn pause(root: &Path, id: &str) -> Result<(Option<String>, Option<Vec<u8>>), String> {
    let user = workloads::username(id);
    let key = Path::new("/etc/ssh/kari_keys").join(&user);
    let bytes = fs::read(&key).ok();
    if bytes.is_some() {
        fs::remove_file(&key).map_err(|e| e.to_string())?;
    }
    let container = current_container(root, id).ok();
    let mut running = None;
    if let Some(name) = container
        && output(
            "docker",
            &["inspect", "--format", "{{.State.Running}}", &name],
        )
        .await?
        .trim()
            == "true"
    {
        run("docker", &["stop", "--time", "10", &name]).await?;
        running = Some(name);
    }
    // Only the site's dedicated identity is terminated, including existing SFTP
    // sessions. The operator's SSH login and other sites are unaffected.
    let result = Command::new("pkill")
        .args(["-KILL", "-u", &user])
        .status()
        .await
        .map_err(|e| e.to_string())?;
    if !result.success() && result.code() != Some(1) {
        return Err("Unable to quiesce site processes".into());
    }
    Ok((running, bytes))
}

async fn resume(
    id: &str,
    domain: &str,
    container: Option<String>,
    key: Option<Vec<u8>>,
) -> Result<(), String> {
    if let Some(name) = container {
        run("docker", &["start", &name]).await?;
        let binding = output("docker", &["port", &name, "3000/tcp"]).await?;
        workloads::ready(
            binding
                .trim()
                .strip_prefix("127.0.0.1:")
                .ok_or("Invalid runtime binding")?,
            domain,
        )
        .await?;
    }
    if let Some(bytes) = key {
        private_write(
            &Path::new("/etc/ssh/kari_keys").join(workloads::username(id)),
            &bytes,
            0o644,
        )?;
    }
    Ok(())
}

async fn database_dump(name: &str, path: &Path) -> Result<(), String> {
    let file = fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(path)
        .map_err(|e| e.to_string())?;
    let mut child = Command::new("docker")
        .args([
            "exec",
            name,
            "pg_dump",
            "-U",
            "postgres",
            "-d",
            "site",
            "--format=custom",
        ])
        .stdout(Stdio::piped())
        .stderr(Stdio::null())
        .kill_on_drop(true)
        .spawn()
        .map_err(|e| e.to_string())?;
    let stdout = child.stdout.take().ok_or("Missing dump output")?;
    use tokio::io::AsyncReadExt;
    let mut limited = stdout.take(512 * 1024 * 1024 + 1);
    let mut archive = tokio::fs::File::from_std(file);
    let size = tokio::io::copy(&mut limited, &mut archive)
        .await
        .map_err(|e| e.to_string())?;
    archive.sync_all().await.map_err(|e| e.to_string())?;
    if size > 512 * 1024 * 1024 {
        child.kill().await.map_err(|e| e.to_string())?;
        return Err("Customer database dump exceeds 512 MiB".into());
    }
    let status = child.wait().await.map_err(|e| e.to_string())?;
    if !status.success() {
        return Err("Customer database dump failed".into());
    }
    Ok(())
}

pub async fn backups(config: &AgentConfig, req: BackupRequest) -> Result<BackupResult, String> {
    let root = bound_site(config, &req.site_id, &req.domain_name)?;
    let backups = root.join("backups");
    fs::create_dir_all(&backups).map_err(|e| e.to_string())?;
    fs::set_permissions(&backups, fs::Permissions::from_mode(0o700)).map_err(|e| e.to_string())?;
    if req.action == "list" {
        let mut result = BackupResult::default();
        for entry in fs::read_dir(&backups).map_err(|e| e.to_string())? {
            if result.backups.len() >= 100 {
                return Err(
                    "At most 100 backups can be listed; remove old backups before creating more"
                        .into(),
                );
            }
            let entry = entry.map_err(|e| e.to_string())?;
            let id = entry.file_name().to_string_lossy().to_string();
            if !uuid(&id) {
                continue;
            }
            if let Ok(bytes) = fs::read(entry.path().join("manifest.json")) {
                let value: Snapshot = serde_json::from_slice(&bytes).map_err(|e| e.to_string())?;
                result.backups.push(BackupRecord {
                    id,
                    created_at: value.created_at,
                });
            }
        }
        result
            .backups
            .sort_by(|a, b| b.created_at.cmp(&a.created_at));
        return Ok(result);
    }
    if !uuid(&req.backup_id) {
        return Err("Invalid backup UUID".into());
    }
    let dir = backups.join(&req.backup_id);
    if req.action == "delete" {
        fs::remove_dir_all(dir).map_err(|e| e.to_string())?;
        return Ok(BackupResult::default());
    }
    if req.action == "create" {
        if fs::read_dir(&backups).map_err(|e| e.to_string())?.count() >= 100 {
            return Err("Remove old backups before creating more".into());
        }
        fs::create_dir(&dir).map_err(|e| e.to_string())?;
        fs::set_permissions(&dir, fs::Permissions::from_mode(0o700)).map_err(|e| e.to_string())?;
        let (container, key) = pause(&root, &req.site_id).await?;
        let capture = async {
            copy_tree(
                &root.join("files"),
                &dir.join("files"),
                &root.join("files"),
                &mut 0,
                &mut 0,
                0,
                false,
            )?;
            let release = fs::canonicalize(root.join("current")).map_err(|e| e.to_string())?;
            if !release.starts_with(root.join("releases")) {
                return Err("Current release escapes its site".into());
            }
            copy_tree(
                &release,
                &dir.join("release"),
                &release,
                &mut 0,
                &mut 0,
                0,
                true,
            )?;
            let runtime = fs::read(root.join("runtime.json"))
                .or_else(|_| fs::read(root.join("source.json")))
                .ok()
                .map(|bytes| {
                    serde_json::from_slice::<RuntimeConfig>(&bytes).map_err(|e| e.to_string())
                })
                .transpose()?;
            let pending_env = fs::read(root.join("runtime-env.json"))
                .ok()
                .map(|bytes| serde_json::from_slice(&bytes).map_err(|e| e.to_string()))
                .transpose()?;
            let database = root.join("database-password").exists();
            if database {
                database_dump(
                    &format!("kari-db-{}", req.site_id),
                    &dir.join("database.dump"),
                )
                .await?;
            }
            let certificate = root.join("tls-current").exists();
            if certificate {
                copy_tree(
                    &root.join("tls-current"),
                    &dir.join("tls"),
                    &root.join("tls-current"),
                    &mut 0,
                    &mut 0,
                    0,
                    false,
                )?;
            }
            let mut snapshot = Snapshot {
                created_at: chrono::Utc::now().to_rfc3339(),
                hashes: HashMap::new(),
                runtime,
                pending_env,
                database,
                certificate,
            };
            hashes(&dir, &dir, &mut snapshot.hashes)?;
            private_write(
                &dir.join("manifest.json"),
                &serde_json::to_vec(&snapshot).map_err(|e| e.to_string())?,
                0o600,
            )?;
            Ok(BackupResult {
                backups: vec![BackupRecord {
                    id: req.backup_id.clone(),
                    created_at: snapshot.created_at,
                }],
                ..Default::default()
            })
        }
        .await;
        resume(&req.site_id, &req.domain_name, container, key).await?;
        if capture.is_err() {
            fs::remove_dir_all(&dir).map_err(|e| e.to_string())?;
        }
        return capture;
    }
    if req.action != "restore" {
        return Err("Unsupported backup action".into());
    }
    let metadata = fs::symlink_metadata(&dir).map_err(|e| e.to_string())?;
    if !metadata.is_dir() || metadata.permissions().mode() & 0o077 != 0 {
        return Err("Backup must be a protected directory".into());
    }
    let snapshot: Snapshot =
        serde_json::from_slice(&fs::read(dir.join("manifest.json")).map_err(|e| e.to_string())?)
            .map_err(|e| e.to_string())?;
    let mut actual = HashMap::new();
    hashes(&dir, &dir, &mut actual)?;
    actual.remove("manifest.json");
    if actual != snapshot.hashes {
        return Err("Backup contents do not match their manifest".into());
    }
    let (_old, key) = pause(&root, &req.site_id).await?;
    // A failed destructive restore leaves application/SFTP stopped. The verified
    // snapshot remains available for an explicit repair/retry by its owner.
    let restore_id = fresh_id()?;
    let release = root.join("releases").join(&restore_id);
    copy_tree(
        &dir.join("release"),
        &release,
        &dir.join("release"),
        &mut 0,
        &mut 0,
        0,
        true,
    )?;
    let new_files = root.join(format!("files-{restore_id}"));
    copy_tree(
        &dir.join("files"),
        &new_files,
        &dir.join("files"),
        &mut 0,
        &mut 0,
        0,
        false,
    )?;
    let retired = root.join(format!("retired-files-{restore_id}"));
    fs::rename(root.join("files"), &retired).map_err(|e| e.to_string())?;
    fs::rename(&new_files, root.join("files")).map_err(|e| e.to_string())?;
    run(
        "chown",
        &[
            "-hRP",
            &workloads::username(&req.site_id),
            root.join("files").to_str().ok_or("Invalid data path")?,
        ],
    )
    .await?;
    let name = format!("kari-db-{}", req.site_id);
    if snapshot.database {
        // Restoring data must not revive a previously revoked database password.
        // The runtime receives a fresh credential through its private env file.
        let password = format!(
            "{}{}",
            fresh_id()?.replace('-', ""),
            fresh_id()?.replace('-', "")
        );
        if !root.join("database-password").exists() {
            workloads::database(
                config,
                DatabaseRequest {
                    site_id: req.site_id.clone(),
                    domain_name: req.domain_name.clone(),
                    action: "create".into(),
                    password: password.clone(),
                },
            )
            .await?;
        }
        let dump = fs::File::open(dir.join("database.dump")).map_err(|e| e.to_string())?;
        let status = Command::new("docker")
            .args([
                "exec",
                "-i",
                &name,
                "pg_restore",
                "-U",
                "postgres",
                "-d",
                "postgres",
                "--clean",
                "--if-exists",
                "--create",
                "--exit-on-error",
            ])
            .stdin(Stdio::from(dump))
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .kill_on_drop(true)
            .status()
            .await
            .map_err(|e| e.to_string())?;
        if !status.success() {
            return Err("Database restore failed; site remains stopped".into());
        }
        workloads::sql(&name, &format!("ALTER ROLE site PASSWORD '{password}';")).await?;
        private_write(&root.join("database-password"), password.as_bytes(), 0o600)?;
    } else if root.join("database-password").exists() {
        workloads::database(
            config,
            DatabaseRequest {
                site_id: req.site_id.clone(),
                domain_name: req.domain_name.clone(),
                action: "delete".into(),
                password: String::new(),
            },
        )
        .await?;
    }
    if snapshot.certificate {
        let tls = root.join(format!("tls-{restore_id}"));
        copy_tree(
            &dir.join("tls"),
            &tls,
            &dir.join("tls"),
            &mut 0,
            &mut 0,
            0,
            false,
        )?;
        fs::set_permissions(&tls, fs::Permissions::from_mode(0o700)).map_err(|e| e.to_string())?;
        fs::set_permissions(tls.join("privkey.pem"), fs::Permissions::from_mode(0o600))
            .map_err(|e| e.to_string())?;
        let next = root.join("tls-next");
        std::os::unix::fs::symlink(&tls, &next).map_err(|e| e.to_string())?;
        fs::rename(next, root.join("tls-current")).map_err(|e| e.to_string())?;
    } else if root.join("tls-current").exists() {
        fs::remove_file(root.join("tls-current")).map_err(|e| e.to_string())?;
    }
    let mut result = BackupResult {
        database_exists: snapshot.database,
        certificate_pem: if snapshot.certificate {
            fs::read(dir.join("tls/fullchain.pem")).map_err(|e| e.to_string())?
        } else {
            Vec::new()
        },
        ..Default::default()
    };
    if let Some(settings) = snapshot
        .runtime
        .as_ref()
        .filter(|settings| settings.runtime == "nodejs")
    {
        private_write(
            &root.join("runtime-env.json"),
            &serde_json::to_vec(snapshot.pending_env.as_ref().unwrap_or(&settings.env))
                .map_err(|e| e.to_string())?,
            0o600,
        )?;
        workloads::start_release(
            config,
            &root,
            &req.site_id,
            &req.domain_name,
            &release,
            &restore_id,
            settings,
        )
        .await?;
        result.runtime = settings.runtime.clone();
        result.repository = settings.repository.clone();
        result.branch = settings.branch.clone();
    } else {
        let next = root.join("next");
        std::os::unix::fs::symlink(&release, &next).map_err(|e| e.to_string())?;
        fs::rename(next, root.join("current")).map_err(|e| e.to_string())?;
        let tls = root.join("tls-current");
        activate_proxy(
            config,
            &req.site_id,
            &req.domain_name,
            if tls.exists() { Some(&tls) } else { None },
        )
        .await?;
        result.runtime = "static".into();
        if let Some(settings) = snapshot.runtime {
            result.repository = settings.repository.clone();
            result.branch = settings.branch.clone();
            private_write(
                &root.join("source.json"),
                &serde_json::to_vec(&settings).map_err(|e| e.to_string())?,
                0o600,
            )?;
        }
    }
    resume(&req.site_id, &req.domain_name, None, key).await?;
    fs::remove_dir_all(retired).map_err(|e| e.to_string())?;
    Ok(result)
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn snapshot_links_cannot_escape_the_saved_release() {
        let temp = tempfile::tempdir().unwrap();
        let source = temp.path().join("source");
        fs::create_dir(&source).unwrap();
        fs::write(source.join("index.js"), "source").unwrap();
        std::os::unix::fs::symlink("../../etc/passwd", source.join("escape")).unwrap();
        assert!(
            copy_tree(
                &source,
                &temp.path().join("copy"),
                &source,
                &mut 0,
                &mut 0,
                0,
                true
            )
            .is_err()
        );
        fs::remove_file(source.join("escape")).unwrap();
        std::os::unix::fs::symlink("index.js", source.join("safe")).unwrap();
        copy_tree(
            &source,
            &temp.path().join("valid"),
            &source,
            &mut 0,
            &mut 0,
            0,
            true,
        )
        .unwrap();
        assert!(
            copy_tree(
                &source,
                &temp.path().join("data"),
                &source,
                &mut 0,
                &mut 0,
                0,
                false
            )
            .is_err()
        );
    }
}
