use crate::sys::traits::ReleaseManager;
use async_trait::async_trait;
use std::path::{Path, PathBuf};
use tokio::fs;

pub struct SystemReleaseManager;

#[async_trait]
impl ReleaseManager for SystemReleaseManager {
    // 🛡️ SOLID: Use strongly typed &Path. Path traversal is prevented upstream by `secure_join`.
    async fn prune_old_releases(
        &self,
        releases_dir: &Path,
        keep_count: usize,
    ) -> Result<usize, String> {
        if !releases_dir.exists() {
            return Ok(0); // Nothing to prune
        }

        // 1. 🛡️ Active Release Resolution (Absolute Physical Path)
        // We use canonicalize() which asks the Linux Kernel to follow all symlinks
        // and return the absolute, physical path (e.g., /var/www/app/releases/20260221141759).
        let base_dir = releases_dir.parent().unwrap_or(releases_dir);
        let current_symlink = base_dir.join("current");

        let active_release_target = fs::canonicalize(&current_symlink)
            .await
            .unwrap_or_else(|_| PathBuf::from("/dev/null/invalid")); // Failsafe if 'current' is broken

        let mut entries = match fs::read_dir(releases_dir).await {
            Ok(dir) => dir,
            Err(e) => return Err(format!("Failed to read releases directory: {}", e)),
        };

        let mut paths: Vec<PathBuf> = Vec::new();

        while let Ok(Some(entry)) = entries.next_entry().await {
            // 2. 🛡️ SLA: Non-Blocking I/O
            // We use entry.file_type().await to fetch metadata asynchronously via Tokio,
            // never blocking the executor thread.
            let file_type = match entry.file_type().await {
                Ok(ft) => ft,
                Err(_) => continue, // Skip unreadable entries
            };

            let path = entry.path();
            let file_name = entry.file_name();
            let name_str = file_name.to_string_lossy();

            // 3. Strict Timestamp Validation
            let is_valid_timestamp =
                name_str.len() == 14 && name_str.chars().all(|c| c.is_ascii_digit());

            if file_type.is_dir() && is_valid_timestamp {
                paths.push(path);
            }
        }

        // Sort paths chronologically (oldest first)
        paths.sort();

        let total_releases = paths.len();
        if total_releases <= keep_count {
            return Ok(0);
        }

        // We slice the array to get only the oldest ones that exceed our keep_count
        let prune_count = total_releases - keep_count;
        let paths_to_delete = &paths[0..prune_count];

        let mut deleted = 0;

        for path in paths_to_delete {
            // 4. 🛡️ The Absolute Safety Check
            // We canonicalize the target path too, ensuring we are comparing apples to apples
            // (absolute physical path to absolute physical path).
            let target_canonical = fs::canonicalize(path)
                .await
                .unwrap_or_else(|_| path.clone());

            if target_canonical == active_release_target {
                tracing::info!(
                    "🛡️ Skipping active release directory from pruning: {:?}",
                    path
                );
                continue;
            }

            if let Err(e) = fs::remove_dir_all(path).await {
                // 5. SLA Observability: Log but do not crash the routine
                tracing::warn!("Failed to delete old release {:?}: {}", path, e);
            } else {
                deleted += 1;
            }
        }

        Ok(deleted)
    }
}
