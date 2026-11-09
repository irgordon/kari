use super::hosting::bound_site;
use crate::config::AgentConfig;
use crate::server::kari_agent::{FileEntry, FileRequest, FileResult};
use nix::fcntl::{OFlag, openat};
use nix::sys::stat::Mode;
use std::fs::File;
use std::io::{Read, Write};
use std::os::fd::{AsRawFd, FromRawFd};
use std::path::Path;

fn open_file(parent: Option<i32>, name: &Path, flags: OFlag) -> Result<File, String> {
    let fd = openat(
        parent,
        name,
        flags | OFlag::O_CLOEXEC | OFlag::O_NOFOLLOW | OFlag::O_NONBLOCK,
        Mode::from_bits_truncate(0o644),
    )
    .map_err(|e| e.to_string())?;
    // openat returned a new descriptor. File becomes its only owner and closes
    // it on every return path; no borrowed descriptor is wrapped here.
    Ok(unsafe { File::from_raw_fd(fd) })
}

fn parent(root: &Path, path: &str) -> Result<(File, String), String> {
    if path.len() > 1024 || path.contains(['\0', '\\']) || path.starts_with('/') {
        return Err("Invalid site-relative path".into());
    }
    let parts: Vec<&str> = path.split('/').collect();
    if parts.len() > 32
        || parts
            .iter()
            .any(|p| p.is_empty() || *p == "." || *p == "..")
    {
        return Err("Use a relative path without empty or parent components".into());
    }
    let mut directory = open_file(None, root, OFlag::O_RDONLY | OFlag::O_DIRECTORY)?;
    for component in &parts[..parts.len() - 1] {
        directory = open_file(
            Some(directory.as_raw_fd()),
            Path::new(component),
            OFlag::O_RDONLY | OFlag::O_DIRECTORY,
        )?;
    }
    Ok((
        directory,
        parts.last().ok_or("Missing filename")?.to_string(),
    ))
}

pub fn files(config: &AgentConfig, req: FileRequest) -> Result<FileResult, String> {
    let root = bound_site(config, &req.site_id, &req.domain_name)?.join("files");
    let mut result = FileResult::default();
    if req.action == "list" {
        let directory = if req.path.is_empty() {
            open_file(None, &root, OFlag::O_RDONLY | OFlag::O_DIRECTORY)?
        } else {
            let (parent, name) = parent(&root, &req.path)?;
            open_file(
                Some(parent.as_raw_fd()),
                Path::new(&name),
                OFlag::O_RDONLY | OFlag::O_DIRECTORY,
            )?
        };
        // The proc path addresses this already opened directory, not an
        // attacker-controlled parent path that SFTP can replace with a symlink.
        for entry in std::fs::read_dir(format!("/proc/self/fd/{}", directory.as_raw_fd()))
            .map_err(|e| e.to_string())?
        {
            let entry = entry.map_err(|e| e.to_string())?;
            if result.entries.len() >= 200 {
                return Err("Directory has more than 200 entries; use SFTP".into());
            }
            let metadata = std::fs::symlink_metadata(entry.path()).map_err(|e| e.to_string())?;
            if !metadata.is_dir() && !metadata.is_file() {
                continue;
            }
            result.entries.push(FileEntry {
                name: entry
                    .file_name()
                    .to_str()
                    .ok_or("Non UTF-8 filename; use SFTP")?
                    .into(),
                directory: metadata.is_dir(),
                size: metadata.len(),
            });
        }
        result.entries.sort_by(|a, b| a.name.cmp(&b.name));
        return Ok(result);
    }
    let (parent, name) = parent(&root, &req.path)?;
    let fd = parent.as_raw_fd();
    match req.action.as_str() {
        "read" => {
            let file = open_file(Some(fd), Path::new(&name), OFlag::O_RDONLY)?;
            if !file.metadata().map_err(|e| e.to_string())?.is_file() {
                return Err("Not an ordinary file".into());
            }
            file.take(262145)
                .read_to_end(&mut result.content)
                .map_err(|e| e.to_string())?;
            if result.content.len() > 262144 {
                return Err("Browser file limit is 256 KiB; use SFTP".into());
            }
        }
        "write" => {
            if req.content.len() > 262144 {
                return Err("Browser file limit is 256 KiB; use SFTP".into());
            }
            let temporary = format!(".kari-write-{}", std::process::id());
            let mut file = open_file(
                Some(fd),
                Path::new(&temporary),
                OFlag::O_WRONLY | OFlag::O_CREAT | OFlag::O_EXCL,
            )?;
            file.write_all(&req.content)
                .and_then(|_| file.sync_all())
                .map_err(|e| e.to_string())?;
            let owner = std::fs::metadata(&root).map_err(|e| e.to_string())?;
            use std::os::unix::fs::MetadataExt;
            nix::unistd::fchown(
                file.as_raw_fd(),
                Some(nix::unistd::Uid::from_raw(owner.uid())),
                Some(nix::unistd::Gid::from_raw(owner.gid())),
            )
            .map_err(|e| e.to_string())?;
            nix::fcntl::renameat(Some(fd), temporary.as_str(), Some(fd), name.as_str())
                .map_err(|e| e.to_string())?;
        }
        "mkdir" => {
            nix::sys::stat::mkdirat(Some(fd), name.as_str(), Mode::from_bits_truncate(0o755))
                .map_err(|e| e.to_string())?;
            let dir = open_file(
                Some(fd),
                Path::new(&name),
                OFlag::O_RDONLY | OFlag::O_DIRECTORY,
            )?;
            use std::os::unix::fs::MetadataExt;
            let owner = std::fs::metadata(&root).map_err(|e| e.to_string())?;
            nix::unistd::fchown(
                dir.as_raw_fd(),
                Some(nix::unistd::Uid::from_raw(owner.uid())),
                Some(nix::unistd::Gid::from_raw(owner.gid())),
            )
            .map_err(|e| e.to_string())?;
        }
        "delete" => {
            nix::unistd::unlinkat(
                Some(fd),
                name.as_str(),
                nix::unistd::UnlinkatFlags::NoRemoveDir,
            )
            .map_err(|e| e.to_string())?;
        }
        _ => return Err("Unsupported file operation; delete removes files only".into()),
    }
    Ok(result)
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn site_paths_do_not_follow_sftp_links() {
        let temp = tempfile::tempdir().unwrap();
        std::os::unix::fs::symlink("/etc", temp.path().join("escape")).unwrap();
        assert!(parent(temp.path(), "escape/passwd").is_err());
        assert!(parent(temp.path(), "../passwd").is_err());
        let (root, name) = parent(temp.path(), "escape").unwrap();
        assert!(open_file(Some(root.as_raw_fd()), Path::new(&name), OFlag::O_RDONLY).is_err());
    }
}
