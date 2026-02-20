// agent/src/sys/cleanup.rs

use crate::sys::traits::ReleaseManager;
use async_trait::async_trait;
use std::path::PathBuf;
use tokio::fs;

pub struct SystemReleaseManager;

#[async_trait]
impl ReleaseManager for SystemReleaseManager {
    async fn prune_old_releases(&self, releases_dir: &str, keep_count: usize) -> Result<usize, String> {
        let mut entries = match fs::read_dir(releases_dir).await {
            Ok(dir) => dir,
            Err(e) => return Err(format!("Failed to read releases directory: {}", e)),
        };

        let mut paths: Vec<PathBuf> = Vec::new();

        // 1. Collect all release directories
        while let Ok(Some(entry)) = entries.next_entry().await {
            let path = entry.path();
            if path.is_dir() {
                paths.push(path);
            }
        }

        // 2. Sort paths alphabetically (which equates to chronological due to our timestamp format)
        paths.sort();

        let total_releases = paths.len();
        if total_releases <= keep_count {
            return Ok(0); // Nothing to prune
        }

        // 3. Calculate how many to delete and slice the array
        let prune_count = total_releases - keep_count;
        let paths_to_delete = &paths[0..prune_count];

        let mut deleted = 0;

        // 4. Safely remove the old directories
        for path in paths_to_delete {
            if let Err(e) = fs::remove_dir_all(path).await {
                // We log the error but don't fail the deployment if one folder is stubborn
                eprintln!("Warning: Failed to delete old release {:?}: {}", path, e);
            } else {
                deleted += 1;
            }
        }

        Ok(deleted)
    }
}
