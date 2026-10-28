//go:build integration

package integration_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// A minimal valid devx.yaml used across multiple tests. Task names are
// top-level keys directly (no tasks:/services:/profiles: wrapper).
const validManifest = `version: 1
project:
  name: my-app
api:
  run: "echo hi"
`

// A manifest that declares the 'go' tool (always present in CI) and a runOnce step.
const manifestWithTools = `version: 1
project:
  name: my-app
api:
  run: "echo hi"
tools:
  - name: go
    check: go version
`

// A manifest with a single setup step that always succeeds.
const manifestWithSetup = `version: 1
project:
  name: my-app
api:
  run: "echo hi"
setup:
  - name: echo-hello
    run: echo hello
`

// ---------------------------------------------------------------------------
// version / help
// ---------------------------------------------------------------------------

func TestVersion(t *testing.T) {
	stdout, _, code := run(t, "", "version")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.HasPrefix(strings.TrimSpace(stdout), "devx") {
		t.Errorf("expected output to start with 'devx', got %q", stdout)
	}
}

func TestHelp(t *testing.T) {
	stdout, _, code := run(t, "", "help")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	for _, want := range []string{"Usage:", "devx setup", "devx validate", "devx doctor"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help output missing %q", want)
		}
	}
}

func TestUnknownCommand(t *testing.T) {
	_, _, code := run(t, "", "notacommand")
	if code == 0 {
		t.Fatal("expected non-zero exit for unknown command")
	}
}

// ---------------------------------------------------------------------------
// validate
// ---------------------------------------------------------------------------

func TestValidate_ValidManifest(t *testing.T) {
	dir := withManifest(t, validManifest)
	stdout, _, code := run(t, dir, "validate")
	if code != 0 {
		t.Fatalf("expected exit 0 for valid manifest, got %d", code)
	}
	if !strings.Contains(stdout, "valid") {
		t.Errorf("expected 'valid' in output, got %q", stdout)
	}
}

func TestValidate_NoTasks(t *testing.T) {
	dir := withManifest(t, `version: 1
project:
  name: my-app
`)
	_, _, code := run(t, dir, "validate")
	if code == 0 {
		t.Fatal("expected non-zero exit for a manifest with no tasks")
	}
}

func TestValidate_WrongVersion(t *testing.T) {
	dir := withManifest(t, `version: 2
project:
  name: my-app
api:
  run: "echo hi"
`)
	_, _, code := run(t, dir, "validate")
	if code == 0 {
		t.Fatal("expected non-zero exit for an unsupported version:")
	}
}

func TestValidate_FileNotFound(t *testing.T) {
	_, _, code := run(t, t.TempDir(), "validate", "--file", "nonexistent.yaml")
	if code == 0 {
		t.Fatal("expected non-zero exit when file not found")
	}
}

func TestValidate_MalformedYAML(t *testing.T) {
	dir := withManifest(t, "{ this is not: yaml: at all [[[")
	_, _, code := run(t, dir, "validate")
	if code == 0 {
		t.Fatal("expected non-zero exit for malformed YAML")
	}
}

func TestValidate_WithTools(t *testing.T) {
	dir := withManifest(t, manifestWithTools)
	_, _, code := run(t, dir, "validate")
	if code != 0 {
		t.Fatalf("expected exit 0 for manifest with tools, got %d", code)
	}
}

func TestValidate_WithSetup(t *testing.T) {
	dir := withManifest(t, manifestWithSetup)
	_, _, code := run(t, dir, "validate")
	if code != 0 {
		t.Fatalf("expected exit 0 for manifest with setup steps, got %d", code)
	}
}

func TestValidate_InvalidToolMissingCheck(t *testing.T) {
	dir := withManifest(t, `version: 1
project:
  name: my-app
api:
  run: "echo hi"
tools:
  - name: mytool
`)
	_, _, code := run(t, dir, "validate")
	if code == 0 {
		t.Fatal("expected non-zero exit when tool is missing 'check' field")
	}
}

func TestValidate_InvalidSetupPlatform(t *testing.T) {
	dir := withManifest(t, `version: 1
project:
  name: my-app
api:
  run: "echo hi"
setup:
  - name: bad-step
    run: echo hi
    platform: beos
`)
	_, _, code := run(t, dir, "validate")
	if code == 0 {
		t.Fatal("expected non-zero exit for invalid platform value")
	}
}

// ---------------------------------------------------------------------------
// setup
// ---------------------------------------------------------------------------

func TestSetup_NoToolsOrSteps_JSON(t *testing.T) {
	dir := withManifest(t, validManifest)
	stdout, _, code := run(t, dir, "setup", "--json")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &results); err != nil {
		t.Fatalf("expected valid JSON array, got %q: %v", stdout, err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for manifest with no tools/setup, got %d", len(results))
	}
}

func TestSetup_ToolPresent_JSON(t *testing.T) {
	// 'go' is always available in the CI environment and on dev machines.
	dir := withManifest(t, manifestWithTools)
	stdout, _, code := run(t, dir, "setup", "--json")
	if code != 0 {
		t.Fatalf("expected exit 0 when tool is present, got %d", code)
	}
	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &results); err != nil {
		t.Fatalf("expected valid JSON, got %q: %v", stdout, err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["name"] != "go" {
		t.Errorf("expected name='go', got %q", results[0]["name"])
	}
	if results[0]["status"] != "ok" {
		t.Errorf("expected status='ok', got %q — is 'go' installed?", results[0]["status"])
	}
}

func TestSetup_StepRuns_JSON(t *testing.T) {
	dir := withManifest(t, manifestWithSetup)
	stdout, _, code := run(t, dir, "setup", "--json")
	if code != 0 {
		t.Fatalf("expected exit 0 when step succeeds, got %d", code)
	}
	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &results); err != nil {
		t.Fatalf("expected valid JSON, got %q: %v", stdout, err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["name"] != "echo-hello" {
		t.Errorf("expected name='echo-hello', got %q", results[0]["name"])
	}
	status, _ := results[0]["status"].(string)
	if status != "ok" && status != "skipped" {
		t.Errorf("expected status 'ok' or 'skipped', got %q", status)
	}
}

func TestSetup_NoManifest(t *testing.T) {
	_, _, code := run(t, t.TempDir(), "setup", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit when devx.yaml is missing")
	}
}

// ---------------------------------------------------------------------------
// doctor
// ---------------------------------------------------------------------------

func TestDoctor_OutputsValidJSON(t *testing.T) {
	dir := withManifest(t, validManifest)
	stdout, _, _ := run(t, dir, "doctor", "--json")
	// Doctor may exit non-zero when Docker is unavailable, but stdout must be valid JSON.
	var checks []map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &checks); err != nil {
		t.Fatalf("expected valid JSON array, got %q: %v", stdout, err)
	}
	if len(checks) == 0 {
		t.Error("expected at least one check in doctor output")
	}
}

func TestDoctor_CLICheckAlwaysPasses(t *testing.T) {
	dir := withManifest(t, validManifest)
	stdout, _, _ := run(t, dir, "doctor", "--json")
	var checks []map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &checks); err != nil {
		t.Fatalf("expected valid JSON: %v", err)
	}
	for _, c := range checks {
		if c["name"] == "CLI" {
			if c["status"] != "PASS" {
				t.Errorf("CLI check should always PASS, got %q", c["status"])
			}
			return
		}
	}
	t.Error("expected a 'CLI' check in doctor output")
}

func TestDoctor_ToolCheck_JSON(t *testing.T) {
	// Manifest includes the 'go' tool which is always available in CI.
	dir := withManifest(t, manifestWithTools)
	stdout, _, _ := run(t, dir, "doctor", "--json")
	var checks []map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &checks); err != nil {
		t.Fatalf("expected valid JSON: %v", err)
	}
	for _, c := range checks {
		if c["name"] == "Tool: go" {
			if c["status"] != "PASS" {
				t.Errorf("expected 'Tool: go' to PASS, got %q", c["status"])
			}
			return
		}
	}
	t.Error("expected a 'Tool: go' check in doctor output")
}

func TestDoctor_NoManifest_StillRuns(t *testing.T) {
	// Without a manifest, doctor should still run runtime checks (just skip tool checks).
	stdout, _, _ := run(t, t.TempDir(), "doctor", "--json")
	var checks []map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &checks); err != nil {
		t.Fatalf("expected valid JSON even without manifest, got %q: %v", stdout, err)
	}
	if len(checks) == 0 {
		t.Error("expected at least one check even without manifest")
	}
}
