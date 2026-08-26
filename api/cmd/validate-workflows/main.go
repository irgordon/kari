package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	actionPolicyPath    = ".github/actions-policy.json"
	goAuthorityInput    = "${{ steps.versions.outputs.go }}"
	nodeAuthorityInput  = "${{ steps.versions.outputs.node }}"
	rustAuthorityInput  = "${{ steps.versions.outputs.rust }}"
	toolchainExportPath = "./scripts/export-toolchain.sh"
)

var (
	immutableSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)
	releaseTag   = regexp.MustCompile(`^v([0-9]+)\.[0-9]+\.[0-9]+$`)
)

type policyFile struct {
	SchemaVersion int              `json:"schema_version"`
	Actions       []approvedAction `json:"actions"`
}

type approvedAction struct {
	Repository   string   `json:"repository"`
	Release      string   `json:"release"`
	SHA          string   `json:"sha"`
	Runtime      string   `json:"runtime"`
	MinimumMajor int      `json:"minimum_major"`
	AllowedPaths []string `json:"allowed_paths"`
	ReleaseURL   string   `json:"release_url"`
}

type actionReference struct {
	Repository string
	Path       string
	SHA        string
}

type workflowValidator struct {
	path    string
	lines   []string
	policy  map[string]approvedAction
	usage   map[string]map[string]int
	usesPin bool
}

func main() {
	if err := validateRepository(requiredRoot()); err != nil {
		fmt.Fprintln(os.Stderr, "workflow validation failed:", err)
		os.Exit(1)
	}
}

func validateRepository(root string) error {
	policy, err := loadActionPolicy(root)
	if err != nil {
		return err
	}
	if err := validateToolchainBridge(root); err != nil {
		return err
	}
	workflows, err := discoverWorkflowFiles(root)
	if err != nil {
		return err
	}
	usage := make(map[string]map[string]int)
	for _, path := range workflows {
		if err := validateWorkflow(path, policy, usage); err != nil {
			return err
		}
	}
	if err := validateLocalActions(root, policy, usage); err != nil {
		return err
	}
	return validatePolicyClosure(policy, usage)
}

func validateWorkflow(path string, policy map[string]approvedAction, usage map[string]map[string]int) error {
	content, document, err := readYAML(path)
	if err != nil {
		return err
	}
	root := document.Content[0]
	if err := validateWorkflowShape(path, root); err != nil {
		return err
	}
	validator := workflowValidator{
		path: path, lines: strings.Split(string(content), "\n"), policy: policy, usage: usage,
	}
	jobs := mappingValue(root, "jobs")
	for index := 0; index < len(jobs.Content); index += 2 {
		jobID := jobs.Content[index].Value
		if err := validator.validateJob(jobID, jobs.Content[index+1]); err != nil {
			return err
		}
	}
	if validator.usesPin && !workflowLoadsToolchains(root) {
		return fmt.Errorf("%s: toolchain setup does not load %s", path, toolchainExportPath)
	}
	return nil
}

func (validator *workflowValidator) validateJob(jobID string, job *yaml.Node) error {
	if uses := mappingValue(job, "uses"); uses != nil {
		return validator.validateUse(jobID, "reusable workflow", uses)
	}
	steps := mappingValue(job, "steps")
	if steps == nil {
		return nil
	}
	for index, step := range steps.Content {
		if err := validator.validateStep(jobID, index, step); err != nil {
			return err
		}
	}
	return nil
}

func (validator *workflowValidator) validateStep(jobID string, index int, step *yaml.Node) error {
	stepName := scalarValue(step, "name")
	if stepName == "" {
		stepName = fmt.Sprintf("step %d", index+1)
	}
	uses := mappingValue(step, "uses")
	if uses != nil {
		if err := validator.validateUse(jobID, stepName, uses); err != nil {
			return err
		}
		return validator.validateSetupInput(jobID, stepName, step, uses.Value)
	}
	return validateRustSetupCommand(validator.path, jobID, stepName, scalarValue(step, "run"))
}

func (validator *workflowValidator) validateUse(jobID string, stepName string, node *yaml.Node) error {
	value := node.Value
	if strings.HasPrefix(value, "./") {
		return nil
	}
	reference, err := parseActionReference(value)
	if err != nil {
		return validator.violation(jobID, stepName, value, err.Error())
	}
	policy, found := validator.policy[reference.Repository]
	if !found {
		return validator.violation(jobID, stepName, value, "action is absent from .github/actions-policy.json")
	}
	if reference.SHA != policy.SHA {
		return validator.violation(jobID, stepName, value,
			fmt.Sprintf("observed SHA %s is not approved; expected SHA %s", reference.SHA, policy.SHA))
	}
	if !contains(policy.AllowedPaths, reference.Path) {
		return validator.violation(jobID, stepName, value, fmt.Sprintf("action path %q is not approved", reference.Path))
	}
	if comment := validator.lineComment(node.Line); comment != policy.Release {
		return validator.violation(jobID, stepName, value, fmt.Sprintf("release comment must be '# %s'", policy.Release))
	}
	recordActionUsage(validator.usage, reference)
	validator.usesPin = true
	return nil
}

func (validator *workflowValidator) validateSetupInput(jobID string, stepName string, step *yaml.Node, action string) error {
	reference, err := parseActionReference(action)
	if err != nil {
		return nil
	}
	switch reference.Repository {
	case "actions/setup-go":
		return requireSetupInput(validator.path, jobID, stepName, step, "go-version", goAuthorityInput)
	case "actions/setup-node":
		return requireSetupInput(validator.path, jobID, stepName, step, "node-version", nodeAuthorityInput)
	default:
		return nil
	}
}

func (validator *workflowValidator) lineComment(lineNumber int) string {
	if lineNumber < 1 || lineNumber > len(validator.lines) {
		return ""
	}
	line := validator.lines[lineNumber-1]
	_, comment, found := strings.Cut(line, "#")
	if !found {
		return ""
	}
	return strings.TrimSpace(comment)
}

func (validator *workflowValidator) violation(jobID string, stepName string, action string, rule string) error {
	return fmt.Errorf("%s: job %q step %q action %q: %s", validator.path, jobID, stepName, action, rule)
}

func loadActionPolicy(root string) (map[string]approvedAction, error) {
	path := filepath.Join(root, actionPolicyPath)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read action policy %s: %w", path, err)
	}
	var file policyFile
	if err := json.Unmarshal(content, &file); err != nil {
		return nil, fmt.Errorf("parse action policy %s: %w", path, err)
	}
	if file.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported action policy schema: %d", file.SchemaVersion)
	}
	return indexPolicies(file.Actions)
}

func indexPolicies(actions []approvedAction) (map[string]approvedAction, error) {
	policies := make(map[string]approvedAction, len(actions))
	for _, action := range actions {
		if err := validatePolicy(action); err != nil {
			return nil, err
		}
		if existing, exists := policies[action.Repository]; exists {
			if existing.SHA != action.SHA {
				return nil, fmt.Errorf("conflicting action policies for %s: SHAs %s and %s", action.Repository, existing.SHA, action.SHA)
			}
			return nil, fmt.Errorf("duplicate action policy for %s", action.Repository)
		}
		policies[action.Repository] = action
	}
	return policies, nil
}

func validatePolicyClosure(policy map[string]approvedAction, usage map[string]map[string]int) error {
	repositories := make([]string, 0, len(policy))
	for repository := range policy {
		repositories = append(repositories, repository)
	}
	sort.Strings(repositories)
	for _, repository := range repositories {
		for _, path := range policy[repository].AllowedPaths {
			if usage[repository][path] == 0 {
				action := repository
				if path != "" {
					action += "/" + path
				}
				return fmt.Errorf("action policy entry %q is unused: no active workflow references it", action)
			}
		}
	}
	return nil
}

func recordActionUsage(usage map[string]map[string]int, reference actionReference) {
	if usage[reference.Repository] == nil {
		usage[reference.Repository] = make(map[string]int)
	}
	usage[reference.Repository][reference.Path]++
}

func validatePolicy(action approvedAction) error {
	if !immutableSHA.MatchString(action.SHA) {
		return fmt.Errorf("action policy %s has invalid SHA", action.Repository)
	}
	major, err := releaseMajor(action.Release)
	if err != nil {
		return fmt.Errorf("action policy %s: %w", action.Repository, err)
	}
	if major < action.MinimumMajor {
		return fmt.Errorf("action policy %s release major %d is below minimum %d", action.Repository, major, action.MinimumMajor)
	}
	if strings.HasPrefix(action.Runtime, "javascript-") && action.Runtime != "javascript-node24" {
		return fmt.Errorf("action policy %s is not Node 24 compatible", action.Repository)
	}
	if action.ReleaseURL != "https://github.com/"+action.Repository+"/releases/tag/"+action.Release {
		return fmt.Errorf("action policy %s has inconsistent release URL", action.Repository)
	}
	return nil
}

func parseActionReference(value string) (actionReference, error) {
	pathAndRepository, reference, found := strings.Cut(value, "@")
	if !found || strings.Contains(reference, "@") {
		return actionReference{}, fmt.Errorf("external action must contain one @ separator")
	}
	if !immutableSHA.MatchString(reference) {
		return actionReference{}, fmt.Errorf("external action reference must be a full 40-character SHA")
	}
	parts := strings.Split(pathAndRepository, "/")
	if len(parts) < 2 {
		return actionReference{}, fmt.Errorf("external action repository is malformed")
	}
	return actionReference{
		Repository: strings.Join(parts[:2], "/"),
		Path:       strings.Join(parts[2:], "/"),
		SHA:        reference,
	}, nil
}

func releaseMajor(release string) (int, error) {
	matches := releaseTag.FindStringSubmatch(release)
	if len(matches) != 2 {
		return 0, fmt.Errorf("invalid release tag %q", release)
	}
	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("invalid release major %q", release)
	}
	return major, nil
}

func validateWorkflowShape(path string, root *yaml.Node) error {
	for _, key := range []string{"name", "on", "permissions", "jobs"} {
		if mappingValue(root, key) == nil {
			return fmt.Errorf("%s has no %s field", path, key)
		}
	}
	jobs := mappingValue(root, "jobs")
	if jobs.Kind != yaml.MappingNode || len(jobs.Content) == 0 {
		return fmt.Errorf("%s has no jobs", path)
	}
	return nil
}

func requireSetupInput(path string, jobID string, stepName string, step *yaml.Node, key string, expected string) error {
	with := mappingValue(step, "with")
	actual := scalarValue(with, key)
	if actual != expected {
		return fmt.Errorf("%s: job %q step %q input %q: expected %q, found %q", path, jobID, stepName, key, expected, actual)
	}
	return nil
}

func validateRustSetupCommand(path string, jobID string, stepName string, command string) error {
	if !strings.Contains(command, "rustup toolchain install") {
		return nil
	}
	if !strings.Contains(command, rustAuthorityInput) {
		return fmt.Errorf("%s: job %q step %q Rust setup must use %q", path, jobID, stepName, rustAuthorityInput)
	}
	return nil
}

func workflowLoadsToolchains(root *yaml.Node) bool {
	jobs := mappingValue(root, "jobs")
	for index := 1; index < len(jobs.Content); index += 2 {
		steps := mappingValue(jobs.Content[index], "steps")
		if steps == nil {
			continue
		}
		for _, step := range steps.Content {
			if scalarValue(step, "id") == "versions" && scalarValue(step, "run") == toolchainExportPath {
				return true
			}
		}
	}
	return false
}

func validateToolchainBridge(root string) error {
	versions, err := readEnvironmentFile(filepath.Join(root, "toolchain.env"))
	if err != nil {
		return err
	}
	for _, key := range []string{"GO_VERSION", "RUST_VERSION", "NODE_VERSION", "NPM_VERSION"} {
		if versions[key] == "" {
			return fmt.Errorf("toolchain.env is missing %s", key)
		}
	}
	content, err := os.ReadFile(filepath.Join(root, "scripts/export-toolchain.sh"))
	if err != nil {
		return fmt.Errorf("read toolchain export script: %w", err)
	}
	for _, output := range []string{"go", "rust", "node", "npm"} {
		if !strings.Contains(string(content), "printf '"+output+"=%s\\n'") {
			return fmt.Errorf("toolchain export script is missing %s output", output)
		}
	}
	return nil
}

func validateLocalActions(root string, policy map[string]approvedAction, usage map[string]map[string]int) error {
	paths, err := discoverLocalActionFiles(root)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if err := validateLocalAction(path, policy, usage); err != nil {
			return err
		}
	}
	return nil
}

func validateLocalAction(path string, policy map[string]approvedAction, usage map[string]map[string]int) error {
	content, document, err := readYAML(path)
	if err != nil {
		return err
	}
	runs := mappingValue(document.Content[0], "runs")
	if scalarValue(runs, "using") != "composite" {
		return nil
	}
	validator := workflowValidator{
		path: path, lines: strings.Split(string(content), "\n"), policy: policy, usage: usage,
	}
	steps := mappingValue(runs, "steps")
	for index, step := range steps.Content {
		if err := validator.validateStep("local-action", index, step); err != nil {
			return err
		}
	}
	return nil
}

func discoverWorkflowFiles(root string) ([]string, error) {
	return discoverFiles(filepath.Join(root, ".github", "workflows"), isYAML)
}

func discoverLocalActionFiles(root string) ([]string, error) {
	return discoverFiles(root, func(path string) bool {
		name := filepath.Base(path)
		return name == "action.yml" || name == "action.yaml"
	})
}

func discoverFiles(root string, match func(string) bool) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && shouldSkipDirectory(entry.Name()) && path != root {
			return filepath.SkipDir
		}
		if !entry.IsDir() && match(path) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover YAML files under %s: %w", root, err)
	}
	sort.Strings(paths)
	return paths, nil
}

func shouldSkipDirectory(name string) bool {
	return name == ".git" || name == ".artifacts" || name == ".runtime" ||
		name == "node_modules" || name == "target" || name == "dist"
}

func isYAML(path string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	return extension == ".yml" || extension == ".yaml"
}

func readYAML(path string) ([]byte, *yaml.Node, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("%s has invalid YAML document shape", path)
	}
	return content, &document, nil
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}
	return nil
}

func scalarValue(mapping *yaml.Node, key string) string {
	value := mappingValue(mapping, key)
	if value == nil || value.Kind != yaml.ScalarNode {
		return ""
	}
	return value.Value
}

func readEnvironmentFile(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	values := make(map[string]string)
	for _, line := range strings.Split(string(content), "\n") {
		key, value, found := strings.Cut(line, "=")
		if found {
			values[key] = value
		}
	}
	return values, nil
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func requiredRoot() string {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: validate-workflows REPOSITORY_ROOT")
		os.Exit(2)
	}
	return os.Args[1]
}
