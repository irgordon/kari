package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseActionReferenceRequiresFullSHA(t *testing.T) {
	for _, reference := range []string{
		"actions/checkout@v7",
		"actions/checkout@3d3c42e",
		"actions/checkout@main",
	} {
		if _, err := parseActionReference(reference); err == nil {
			t.Fatalf("parseActionReference(%q) unexpectedly succeeded", reference)
		}
	}
}

func TestValidateUseReportsContext(t *testing.T) {
	const action = "actions/checkout@11d5960a326750d5838078e36cf38b85af677262"
	validator := workflowValidator{
		path:   ".github/workflows/verify.yml",
		lines:  []string{"uses: " + action + " # v4.3.1"},
		policy: map[string]approvedAction{"actions/checkout": checkoutPolicy()},
		usage:  make(map[string]map[string]int),
	}
	node := &yaml.Node{Kind: yaml.ScalarNode, Value: action, Line: 1}
	err := validator.validateUse("verify", "Checkout repository", node)
	if err == nil {
		t.Fatal("validateUse() unexpectedly succeeded")
	}
	for _, fragment := range []string{"verify.yml", "verify", "Checkout repository", action, "expected"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("validateUse() error %q is missing %q", err, fragment)
		}
	}
}

func TestValidateUseAcceptsPolicySHA(t *testing.T) {
	policy := checkoutPolicy()
	action := policy.Repository + "@" + policy.SHA
	validator := workflowValidator{
		path:   ".github/workflows/verify.yml",
		lines:  []string{"uses: " + action + " # " + policy.Release},
		policy: map[string]approvedAction{policy.Repository: policy},
		usage:  make(map[string]map[string]int),
	}
	node := &yaml.Node{Kind: yaml.ScalarNode, Value: action, Line: 1}
	if err := validator.validateUse("verify", "Checkout repository", node); err != nil {
		t.Fatalf("validateUse() error = %v", err)
	}
}

func TestPolicyClosureScenarios(t *testing.T) {
	checkout := checkoutPolicy()
	codeql := codeQLPolicy()
	otherSHA := strings.Repeat("a", 40)
	tests := []struct {
		name      string
		uses      []workflowUse
		policies  []approvedAction
		local     bool
		wantError []string
	}{
		{"matching policy", []workflowUse{externalUse(checkout, "")}, []approvedAction{checkout}, false, nil},
		{"missing policy", []workflowUse{externalUse(checkout, "")}, nil, false, []string{"Use actions/checkout/", "absent from"}},
		{"unused policy", []workflowUse{externalUse(checkout, "")}, []approvedAction{checkout, setupGoPolicy()}, false, []string{"actions/setup-go", "unused", "no active workflow"}},
		{"duplicate policy", []workflowUse{externalUse(checkout, "")}, []approvedAction{checkout, checkout}, false, []string{"duplicate action policy"}},
		{"conflicting policy", []workflowUse{externalUse(checkout, "")}, []approvedAction{checkout, withSHA(checkout, otherSHA)}, false, []string{"conflicting action policies", otherSHA}},
		{"mutable major", []workflowUse{{Name: "Checkout", Reference: "actions/checkout@v7", Comment: checkout.Release}}, []approvedAction{checkout}, false, []string{"workflow.yml", "job", "Checkout", "full 40-character SHA"}},
		{"branch reference", []workflowUse{{Name: "Checkout", Reference: "actions/checkout@main", Comment: checkout.Release}}, []approvedAction{checkout}, false, []string{"actions/checkout@main", "full 40-character SHA"}},
		{"short SHA", []workflowUse{{Name: "Checkout", Reference: "actions/checkout@3d3c42e", Comment: checkout.Release}}, []approvedAction{checkout}, false, []string{"actions/checkout@3d3c42e", "full 40-character SHA"}},
		{"unknown full SHA", []workflowUse{{Name: "Checkout", Reference: "actions/checkout@" + otherSHA, Comment: checkout.Release}}, []approvedAction{checkout}, false, []string{"observed", otherSHA, "expected", checkout.SHA}},
		{"local action", []workflowUse{{Name: "Local", Reference: "./.github/actions/local"}}, nil, true, nil},
		{"CodeQL subactions", []workflowUse{externalUse(codeql, "init"), externalUse(codeql, "analyze")}, []approvedAction{codeql}, false, nil},
		{"unused CodeQL subaction", []workflowUse{externalUse(codeql, "init"), externalUse(codeql, "analyze")}, []approvedAction{withAllowedPaths(codeql, "init", "analyze", "autobuild")}, false, []string{"github/codeql-action/autobuild", "unused"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := policyTestRepository(t, test.uses, test.policies, test.local)
			err := validateRepository(repository)
			assertPolicyResult(t, err, test.wantError)
		})
	}
}

type workflowUse struct {
	Name      string
	Reference string
	Comment   string
}

func policyTestRepository(t *testing.T, uses []workflowUse, policies []approvedAction, local bool) string {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, root, "toolchain.env", "GO_VERSION=1.27.0\nRUST_VERSION=1.98.0\nNODE_VERSION=24.19.0\nNPM_VERSION=11.17.0\n")
	writeTestFile(t, root, "scripts/export-toolchain.sh", "printf 'go=%s\\n'\nprintf 'rust=%s\\n'\nprintf 'node=%s\\n'\nprintf 'npm=%s\\n'\n")
	writeTestFile(t, root, ".github/workflows/workflow.yml", renderWorkflow(uses))
	writePolicyFile(t, root, policies)
	if local {
		writeTestFile(t, root, ".github/actions/local/action.yml", "name: Local\nruns:\n  using: composite\n  steps:\n    - shell: bash\n      run: echo local\n")
	}
	return root
}

func renderWorkflow(uses []workflowUse) string {
	var workflow strings.Builder
	workflow.WriteString("name: Test\non:\n  push:\npermissions:\n  contents: read\njobs:\n  job:\n    runs-on: ubuntu-24.04\n    steps:\n")
	workflow.WriteString("      - name: Load toolchains\n        id: versions\n        run: ./scripts/export-toolchain.sh\n")
	for _, use := range uses {
		fmt.Fprintf(&workflow, "      - name: %s\n        uses: %s", use.Name, use.Reference)
		if use.Comment != "" {
			fmt.Fprintf(&workflow, " # %s", use.Comment)
		}
		workflow.WriteByte('\n')
	}
	return workflow.String()
}

func writePolicyFile(t *testing.T, root string, policies []approvedAction) {
	t.Helper()
	content, err := json.Marshal(policyFile{SchemaVersion: 1, Actions: policies})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, actionPolicyPath, string(content))
}

func writeTestFile(t *testing.T, root string, relativePath string, content string) {
	t.Helper()
	path := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertPolicyResult(t *testing.T, err error, expectedFragments []string) {
	t.Helper()
	if len(expectedFragments) == 0 {
		if err != nil {
			t.Fatalf("validateRepository() error = %v", err)
		}
		return
	}
	if err == nil {
		t.Fatal("validateRepository() unexpectedly succeeded")
	}
	for _, fragment := range expectedFragments {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error %q is missing %q", err, fragment)
		}
	}
}

func externalUse(policy approvedAction, path string) workflowUse {
	reference := policy.Repository
	if path != "" {
		reference += "/" + path
	}
	return workflowUse{Name: "Use " + policy.Repository + "/" + path, Reference: reference + "@" + policy.SHA, Comment: policy.Release}
}

func withSHA(policy approvedAction, sha string) approvedAction {
	policy.SHA = sha
	return policy
}

func withAllowedPaths(policy approvedAction, paths ...string) approvedAction {
	policy.AllowedPaths = paths
	return policy
}

func checkoutPolicy() approvedAction {
	return approvedAction{
		Repository: "actions/checkout", Release: "v7.0.1",
		SHA:     "3d3c42e5aac5ba805825da76410c181273ba90b1",
		Runtime: "javascript-node24", MinimumMajor: 7,
		AllowedPaths: []string{""}, ReleaseURL: "https://github.com/actions/checkout/releases/tag/v7.0.1",
	}
}

func setupGoPolicy() approvedAction {
	return approvedAction{
		Repository: "actions/setup-go", Release: "v7.0.0",
		SHA:     "b7ad1dad31e06c5925ef5d2fc7ad053ef454303e",
		Runtime: "javascript-node24", MinimumMajor: 7,
		AllowedPaths: []string{""}, ReleaseURL: "https://github.com/actions/setup-go/releases/tag/v7.0.0",
	}
}

func codeQLPolicy() approvedAction {
	return approvedAction{
		Repository: "github/codeql-action", Release: "v4.37.8",
		SHA:     "db488ddef3bf6cb639b32c2e9a7c0a7ea8271d28",
		Runtime: "javascript-node24", MinimumMajor: 4,
		AllowedPaths: []string{"init", "analyze"}, ReleaseURL: "https://github.com/github/codeql-action/releases/tag/v4.37.8",
	}
}
