package sourceidentity

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIdentityPortableAcrossLinkedWorktrees(t *testing.T) {
	repository := newTestRepository(t)
	worktreeParent := t.TempDir()
	first := filepath.Join(worktreeParent, "first-location")
	second := filepath.Join(worktreeParent, "second-location")
	gitCommand(t, repository, "worktree", "add", "--detach", first, "HEAD")
	gitCommand(t, repository, "worktree", "add", "--detach", second, "HEAD")
	t.Cleanup(func() {
		gitCommand(t, repository, "worktree", "remove", "--force", first)
		gitCommand(t, repository, "worktree", "remove", "--force", second)
	})
	applyIdenticalProposal(t, first)
	applyIdenticalProposal(t, second)
	firstPointer := readFile(t, filepath.Join(first, ".git"))
	secondPointer := readFile(t, filepath.Join(second, ".git"))
	if bytes.Equal(firstPointer, secondPointer) {
		t.Fatal("linked-worktree .git pointers unexpectedly match")
	}
	assertPortableIdentity(t, first, second)
}

func TestIdentityChangesForGitTreeMutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{"content", func(t *testing.T, root string) { writeFile(t, root, "regular.txt", "changed\n", 0o644) }},
		{"addition", func(t *testing.T, root string) { writeFile(t, root, "added.txt", "added\n", 0o644) }},
		{"deletion", func(t *testing.T, root string) { removeFile(t, filepath.Join(root, "delete-me.txt")) }},
		{"rename", func(t *testing.T, root string) {
			renameFile(t, filepath.Join(root, "rename-me.txt"), filepath.Join(root, "renamed.txt"))
		}},
		{"executable mode", func(t *testing.T, root string) { chmodFile(t, filepath.Join(root, "mode.txt"), 0o755) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := newTestRepository(t)
			before := generateIdentity(t, repository)
			test.mutate(t, repository)
			after := generateIdentity(t, repository)
			if before.TreeID == after.TreeID || before.ManifestSHA256 == after.ManifestSHA256 {
				t.Fatalf("%s did not change both identities", test.name)
			}
		})
	}
}

func TestIgnoredEvidenceDoesNotChangeIdentity(t *testing.T) {
	repository := newTestRepository(t)
	before := generateIdentity(t, repository)
	writeFile(t, repository, ".artifacts/evidence.txt", "ignored evidence\n", 0o644)
	after := generateIdentity(t, repository)
	if before.TreeID != after.TreeID || before.ManifestSHA256 != after.ManifestSHA256 {
		t.Fatal("ignored .artifacts content changed source identity")
	}
}

func TestSymlinkUsesStoredTargetBlob(t *testing.T) {
	repository := newTestRepository(t)
	result := generateIdentity(t, repository)
	entry, found := findEntry(result.Entries, "regular-link")
	if !found {
		t.Fatal("symlink entry is absent")
	}
	if entry.Mode != "120000" || entry.Type != "blob" {
		t.Fatalf("symlink entry = %#v", entry)
	}
	digest := sha256.Sum256([]byte("regular.txt"))
	if entry.ContentSHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("symlink digest = %s", entry.ContentSHA256)
	}
}

func assertPortableIdentity(t *testing.T, first string, second string) {
	t.Helper()
	temporaryIndexes := t.TempDir()
	firstManifest := filepath.Join(t.TempDir(), "first.manifest")
	secondManifest := filepath.Join(t.TempDir(), "second.manifest")
	firstIndex := readRealIndex(t, first)
	secondIndex := readRealIndex(t, second)
	firstBranch, firstHead := branchAndHead(t, first)
	secondBranch, secondHead := branchAndHead(t, second)
	firstResult := generateWithOptions(t, first, firstManifest, temporaryIndexes)
	assertDirectoryEmpty(t, temporaryIndexes)
	secondResult := generateWithOptions(t, second, secondManifest, temporaryIndexes)
	assertDirectoryEmpty(t, temporaryIndexes)
	if firstResult.TreeID != secondResult.TreeID || firstResult.ManifestSHA256 != secondResult.ManifestSHA256 {
		t.Fatalf("portable identities differ: %#v %#v", firstResult, secondResult)
	}
	if !bytes.Equal(readFile(t, firstManifest), readFile(t, secondManifest)) {
		t.Fatal("portable manifest bytes differ")
	}
	assertEntryPresent(t, firstResult.Entries, "proposed.txt")
	assertEntryAbsent(t, firstResult.Entries, ".git")
	repeated := generateWithOptions(t, first, filepath.Join(t.TempDir(), "repeated.manifest"), temporaryIndexes)
	if firstResult.TreeID != repeated.TreeID || firstResult.ManifestSHA256 != repeated.ManifestSHA256 {
		t.Fatal("repeated identity changed")
	}
	if !bytes.Equal(firstIndex, readRealIndex(t, first)) || !bytes.Equal(secondIndex, readRealIndex(t, second)) {
		t.Fatal("real Git index changed")
	}
	assertBranchAndHead(t, first, firstBranch, firstHead)
	assertBranchAndHead(t, second, secondBranch, secondHead)
}

func newTestRepository(t *testing.T) string {
	t.Helper()
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, repository, "init", "--quiet")
	gitCommand(t, repository, "config", "user.email", "identity@example.test")
	gitCommand(t, repository, "config", "user.name", "Identity Test")
	gitCommand(t, repository, "config", "commit.gpgsign", "false")
	writeFile(t, repository, ".gitignore", ".artifacts/\n", 0o644)
	writeFile(t, repository, "regular.txt", "regular\n", 0o644)
	writeFile(t, repository, "delete-me.txt", "delete\n", 0o644)
	writeFile(t, repository, "rename-me.txt", "rename\n", 0o644)
	writeFile(t, repository, "mode.txt", "mode\n", 0o644)
	if err := os.Symlink("regular.txt", filepath.Join(repository, "regular-link")); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, repository, "add", "-A")
	gitCommand(t, repository, "commit", "--quiet", "-m", "fixture")
	return repository
}

func applyIdenticalProposal(t *testing.T, root string) {
	t.Helper()
	writeFile(t, root, "regular.txt", "proposed regular\n", 0o644)
	writeFile(t, root, "proposed.txt", "intended untracked source\n", 0o644)
	chmodFile(t, filepath.Join(root, "mode.txt"), 0o755)
}

func generateIdentity(t *testing.T, repository string) Result {
	t.Helper()
	return generateWithOptions(t, repository, filepath.Join(t.TempDir(), "manifest"), t.TempDir())
}

func generateWithOptions(t *testing.T, repository string, manifest string, tempParent string) Result {
	t.Helper()
	result, err := GenerateWithOptions(repository, manifest, Options{TempParent: tempParent})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func readRealIndex(t *testing.T, repository string) []byte {
	t.Helper()
	indexPath := strings.TrimSpace(gitCommand(t, repository, "rev-parse", "--git-path", "index"))
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(repository, indexPath)
	}
	return readFile(t, indexPath)
}

func branchAndHead(t *testing.T, repository string) (string, string) {
	t.Helper()
	return strings.TrimSpace(gitCommand(t, repository, "rev-parse", "--abbrev-ref", "HEAD")),
		strings.TrimSpace(gitCommand(t, repository, "rev-parse", "HEAD"))
}

func assertBranchAndHead(t *testing.T, repository string, branch string, head string) {
	t.Helper()
	afterBranch, afterHead := branchAndHead(t, repository)
	if afterBranch != branch || afterHead != head {
		t.Fatalf("branch/HEAD changed: %s %s", afterBranch, afterHead)
	}
}

func gitCommand(t *testing.T, repository string, arguments ...string) string {
	t.Helper()
	commandArguments := append([]string{"-C", repository}, arguments...)
	command := exec.Command("git", commandArguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %s: %v", strings.Join(arguments, " "), output, err)
	}
	return string(output)
}

func writeFile(t *testing.T, root string, relativePath string, content string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func removeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func renameFile(t *testing.T, oldPath string, newPath string) {
	t.Helper()
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
}

func chmodFile(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func assertDirectoryEmpty(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary identity directory is not empty: %v", entries)
	}
}

func findEntry(entries []Entry, path string) (Entry, bool) {
	for _, entry := range entries {
		if entry.Path == path {
			return entry, true
		}
	}
	return Entry{}, false
}

func assertEntryPresent(t *testing.T, entries []Entry, path string) {
	t.Helper()
	if _, found := findEntry(entries, path); !found {
		t.Fatalf("manifest is missing %q", path)
	}
}

func assertEntryAbsent(t *testing.T, entries []Entry, path string) {
	t.Helper()
	if _, found := findEntry(entries, path); found {
		t.Fatalf("manifest unexpectedly contains %q", path)
	}
}
