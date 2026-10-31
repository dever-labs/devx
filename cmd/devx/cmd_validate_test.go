package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/dever-labs/devx/internal/config"
)

func TestRunValidate_ValidManifest(t *testing.T) {
	writeManifest(t, validManifest)
	if err := runValidate(nil); err != nil {
		t.Fatalf("runValidate: %v", err)
	}
}

func TestRunValidate_MissingManifest(t *testing.T) {
	chdirTemp(t)
	if err := runValidate(nil); err == nil {
		t.Fatal("expected error for missing devx.yaml")
	}
}

func TestRunValidate_MalformedYAML(t *testing.T) {
	writeManifest(t, "not: valid: yaml: [")
	if err := runValidate(nil); err == nil {
		t.Fatal("expected error for malformed devx.yaml")
	}
}

func TestRunValidate_FileFlag(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("custom.yaml", []byte(validManifest), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runValidate([]string{"--file", "custom.yaml"}); err != nil {
		t.Fatalf("runValidate with --file: %v", err)
	}
}

func TestRunValidate_TaskMissingRun(t *testing.T) {
	writeManifest(t, `version: 1
project:
  name: my-app
api: {}
`)
	if err := runValidate(nil); err == nil {
		t.Fatal("expected error for task missing run:")
	}
}

func TestExtendedWarnings_BuiltinNameCollision(t *testing.T) {
	m := mustLoad(t, `version: 1
project:
  name: my-app
up:
  run: "echo hi"
`)
	warnings := extendedWarnings(m)
	if !containsSubstring(warnings, "collides with a built-in command name") {
		t.Errorf("expected a built-in-collision warning, got %v", warnings)
	}
}

func TestExtendedWarnings_PortCollision(t *testing.T) {
	m := mustLoad(t, `version: 1
project:
  name: my-app
api:
  run: "nc -l 8080"
  port: 8080
web:
  run: "nc -l 8080"
  port: 8080
`)
	warnings := extendedWarnings(m)
	if !containsSubstring(warnings, "is used by multiple tasks") {
		t.Errorf("expected a port-collision warning, got %v", warnings)
	}
}

func TestExtendedWarnings_NoWarningsForCleanManifest(t *testing.T) {
	m := mustLoad(t, validManifest)
	if warnings := extendedWarnings(m); len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

// TestExtendedWarnings_OneShotContainerNoNameWarning is a regression test
// for issue #58: a one-shot container (e.g. `docker run --rm image`, no
// -d/--detach) was flagged for "no container name could be parsed" even
// though nothing persists for devx down/exec to act on — there's no name
// to parse because the container never outlives the run: command.
func TestExtendedWarnings_OneShotContainerNoNameWarning(t *testing.T) {
	m := mustLoad(t, `version: 1
project:
  name: my-app
migrate:
  run: "docker run --rm my-image migrate"
`)
	warnings := extendedWarnings(m)
	if containsSubstring(warnings, "no name could be parsed") {
		t.Errorf("expected no container-name warning for a one-shot container, got %v", warnings)
	}
}

// TestExtendedWarnings_ConflictsSuppressesPortCollision is a regression
// test for issue #58: two tasks intentionally sharing a port (declared
// via conflicts:, e.g. mutually-exclusive dev-vs-prod variants) should
// not be reported as a port collision.
func TestExtendedWarnings_ConflictsSuppressesPortCollision(t *testing.T) {
	m := mustLoad(t, `version: 1
project:
  name: my-app
api:
  run: "nc -l 8080"
  port: 8080
  conflicts: [api-prod]
api-prod:
  run: "nc -l 8080"
  port: 8080
  conflicts: [api]
`)
	warnings := extendedWarnings(m)
	if containsSubstring(warnings, "is used by multiple tasks") {
		t.Errorf("expected conflicts: to suppress the port-collision warning, got %v", warnings)
	}
}

// TestRunValidate_WarningsDoNotFail is a regression test for issue #58:
// extendedWarnings() findings (e.g. a built-in-name collision) must not
// cause runValidate to return an error / exit non-zero — only hard
// issues (config.Validate et al.) should fail validation.
func TestRunValidate_WarningsDoNotFail(t *testing.T) {
	writeManifest(t, `version: 1
project:
  name: my-app
up:
  run: "echo hi"
`)
	if err := runValidate(nil); err != nil {
		t.Errorf("runValidate should not fail on warnings-only manifest, got %v", err)
	}
}

func TestRunValidate_JSON_ValidManifest(t *testing.T) {
	writeManifest(t, validManifest)
	var validateErr error
	out := captureStdout(t, func() {
		validateErr = runValidate([]string{"--json"})
	})
	if validateErr != nil {
		t.Fatalf("runValidate --json: %v", validateErr)
	}
	var result struct {
		Valid  bool     `json:"valid"`
		Issues []string `json:"issues"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("runValidate --json produced invalid JSON: %v\noutput: %s", err, out)
	}
	if !result.Valid || len(result.Issues) != 0 {
		t.Errorf("expected valid=true with no issues, got %+v", result)
	}
}

func TestRunValidate_JSON_InvalidManifest(t *testing.T) {
	writeManifest(t, `version: 1
project:
  name: my-app
api: {}
`)
	var validateErr error
	out := captureStdout(t, func() {
		validateErr = runValidate([]string{"--json"})
	})
	if validateErr == nil {
		t.Fatal("expected error for task missing run:")
	}
	var result struct {
		Valid  bool     `json:"valid"`
		Issues []string `json:"issues"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("runValidate --json produced invalid JSON: %v\noutput: %s", err, out)
	}
	if result.Valid || len(result.Issues) == 0 {
		t.Errorf("expected valid=false with issues, got %+v", result)
	}
}

func TestRunValidate_JSON_WarningsFieldPopulated(t *testing.T) {
	writeManifest(t, `version: 1
project:
  name: my-app
up:
  run: "echo hi"
`)
	var validateErr error
	out := captureStdout(t, func() {
		validateErr = runValidate([]string{"--json"})
	})
	if validateErr != nil {
		t.Fatalf("runValidate --json should not fail on warnings-only manifest: %v", validateErr)
	}
	var result struct {
		Valid    bool     `json:"valid"`
		Issues   []string `json:"issues"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("runValidate --json produced invalid JSON: %v\noutput: %s", err, out)
	}
	if !result.Valid || len(result.Issues) != 0 {
		t.Errorf("expected valid=true with no issues, got %+v", result)
	}
	if !containsSubstring(result.Warnings, "collides with a built-in command name") {
		t.Errorf("expected warnings to include the built-in-collision warning, got %+v", result.Warnings)
	}
}

func mustLoad(t *testing.T, content string) *config.Manifest {
	t.Helper()
	writeManifest(t, content)
	m, err := loadManifest()
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	return m
}

func containsSubstring(items []string, substr string) bool {
	for _, item := range items {
		if strings.Contains(item, substr) {
			return true
		}
	}
	return false
}
