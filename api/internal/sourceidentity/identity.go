package sourceidentity

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const Algorithm = "kari-source-identity-v2"

type Options struct {
	TempParent string
}

type Entry struct {
	Path          string `json:"path"`
	Mode          string `json:"mode"`
	Type          string `json:"type"`
	ContentSHA256 string `json:"content_sha256"`
}

type Result struct {
	Algorithm      string  `json:"algorithm"`
	ObjectFormat   string  `json:"git_object_format"`
	TreeID         string  `json:"git_tree_id"`
	ManifestSHA256 string  `json:"manifest_sha256"`
	ManifestPath   string  `json:"manifest_path"`
	EntryCount     int     `json:"entry_count"`
	Entries        []Entry `json:"-"`
}

type repositoryState struct {
	IndexPath   string
	IndexDigest string
	Branch      string
	Head        string
}

func Generate(repositoryRoot string, manifestPath string) (Result, error) {
	return GenerateWithOptions(repositoryRoot, manifestPath, Options{})
}

func GenerateWithOptions(repositoryRoot string, manifestPath string, options Options) (Result, error) {
	root, err := resolveRepositoryRoot(repositoryRoot)
	if err != nil {
		return Result{}, err
	}
	before, err := captureRepositoryState(root)
	if err != nil {
		return Result{}, err
	}
	result, err := generateFromTemporaryIndex(root, manifestPath, options)
	if err != nil {
		return Result{}, err
	}
	if err := verifyRepositoryState(root, before); err != nil {
		return Result{}, err
	}
	return result, nil
}

func generateFromTemporaryIndex(root string, manifestPath string, options Options) (Result, error) {
	temporaryDirectory, err := os.MkdirTemp(options.TempParent, Algorithm+"-")
	if err != nil {
		return Result{}, fmt.Errorf("create temporary identity directory: %w", err)
	}
	defer os.RemoveAll(temporaryDirectory)
	indexPath := filepath.Join(temporaryDirectory, "index")
	treeID, err := writeProposedTree(root, indexPath)
	if err != nil {
		return Result{}, err
	}
	entries, err := readTreeEntries(root, treeID)
	if err != nil {
		return Result{}, err
	}
	manifest := encodeManifest(entries)
	if err := writeManifest(manifestPath, manifest); err != nil {
		return Result{}, err
	}
	objectFormat, err := gitOutput(root, nil, "rev-parse", "--show-object-format")
	if err != nil {
		return Result{}, err
	}
	return Result{
		Algorithm: Algorithm, ObjectFormat: objectFormat, TreeID: treeID,
		ManifestSHA256: sha256Hex(manifest), ManifestPath: manifestPath,
		EntryCount: len(entries), Entries: entries,
	}, nil
}

func writeProposedTree(root string, indexPath string) (string, error) {
	environment := map[string]string{"GIT_INDEX_FILE": indexPath}
	if _, err := gitOutput(root, environment, "read-tree", "HEAD"); err != nil {
		return "", fmt.Errorf("initialize temporary index: %w", err)
	}
	if _, err := gitOutput(root, environment, "add", "-A", "--", "."); err != nil {
		return "", fmt.Errorf("populate temporary index: %w", err)
	}
	treeID, err := gitOutput(root, environment, "write-tree")
	if err != nil {
		return "", fmt.Errorf("write proposed Git tree: %w", err)
	}
	return treeID, nil
}

func readTreeEntries(root string, treeID string) ([]Entry, error) {
	listing, err := gitBytes(root, nil, "ls-tree", "-r", "-z", "--full-tree", treeID)
	if err != nil {
		return nil, fmt.Errorf("list proposed Git tree: %w", err)
	}
	entries, err := parseTreeEntries(root, listing)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(left int, right int) bool {
		return bytes.Compare([]byte(entries[left].Path), []byte(entries[right].Path)) < 0
	})
	return entries, nil
}

func parseTreeEntries(root string, listing []byte) ([]Entry, error) {
	records := bytes.Split(listing, []byte{0})
	entries := make([]Entry, 0, len(records))
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		entry, err := parseTreeEntry(root, record)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func parseTreeEntry(root string, record []byte) (Entry, error) {
	header, path, found := bytes.Cut(record, []byte{'\t'})
	if !found {
		return Entry{}, fmt.Errorf("invalid Git tree record")
	}
	fields := bytes.Fields(header)
	if len(fields) != 3 {
		return Entry{}, fmt.Errorf("invalid Git tree header for %q", path)
	}
	objectType := string(fields[1])
	if objectType == "commit" {
		return Entry{}, fmt.Errorf("submodule or Git link %q requires an explicit identity policy", path)
	}
	if objectType != "blob" {
		return Entry{}, fmt.Errorf("unsupported Git object type %q for %q", objectType, path)
	}
	content, err := gitBytes(root, nil, "cat-file", "blob", string(fields[2]))
	if err != nil {
		return Entry{}, fmt.Errorf("read Git blob for %q: %w", path, err)
	}
	return Entry{
		Path: string(path), Mode: string(fields[0]), Type: objectType,
		ContentSHA256: sha256Hex(content),
	}, nil
}

func encodeManifest(entries []Entry) []byte {
	var manifest bytes.Buffer
	writeField(&manifest, Algorithm)
	writeField(&manifest, "path")
	writeField(&manifest, "mode")
	writeField(&manifest, "type")
	writeField(&manifest, "sha256")
	for _, entry := range entries {
		writeField(&manifest, entry.Path)
		writeField(&manifest, entry.Mode)
		writeField(&manifest, entry.Type)
		writeField(&manifest, entry.ContentSHA256)
	}
	return manifest.Bytes()
}

func writeField(manifest *bytes.Buffer, value string) {
	manifest.WriteString(value)
	manifest.WriteByte(0)
}

func writeManifest(path string, manifest []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create manifest directory: %w", err)
	}
	if err := os.WriteFile(path, manifest, 0o644); err != nil {
		return fmt.Errorf("write identity manifest: %w", err)
	}
	return nil
}

func captureRepositoryState(root string) (repositoryState, error) {
	indexPath, err := gitOutput(root, nil, "rev-parse", "--git-path", "index")
	if err != nil {
		return repositoryState{}, err
	}
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(root, indexPath)
	}
	indexContent, err := os.ReadFile(indexPath)
	if err != nil {
		return repositoryState{}, fmt.Errorf("read real Git index: %w", err)
	}
	branch, err := gitOutput(root, nil, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return repositoryState{}, err
	}
	head, err := gitOutput(root, nil, "rev-parse", "HEAD")
	if err != nil {
		return repositoryState{}, err
	}
	return repositoryState{
		IndexPath: indexPath, IndexDigest: sha256Hex(indexContent), Branch: branch, Head: head,
	}, nil
}

func verifyRepositoryState(root string, before repositoryState) error {
	after, err := captureRepositoryState(root)
	if err != nil {
		return err
	}
	if after.IndexPath != before.IndexPath || after.IndexDigest != before.IndexDigest {
		return fmt.Errorf("real Git index changed during source identity generation")
	}
	if after.Branch != before.Branch {
		return fmt.Errorf("branch changed during source identity generation")
	}
	if after.Head != before.Head {
		return fmt.Errorf("HEAD changed during source identity generation")
	}
	return nil
}

func resolveRepositoryRoot(candidate string) (string, error) {
	root, err := gitOutput(candidate, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return root, nil
}

func gitOutput(root string, environment map[string]string, arguments ...string) (string, error) {
	output, err := gitBytes(root, environment, arguments...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func gitBytes(root string, environment map[string]string, arguments ...string) ([]byte, error) {
	commandArguments := append([]string{"-C", root}, arguments...)
	command := exec.Command("git", commandArguments...)
	command.Env = os.Environ()
	for key, value := range environment {
		command.Env = append(command.Env, key+"="+value)
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %s: %w", strings.Join(arguments, " "), strings.TrimSpace(string(output)), err)
	}
	return output, nil
}

func sha256Hex(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}
