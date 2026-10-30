package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRunProjectSetup_MissingManifestErrors(t *testing.T) {
	chdirTemp(t)
	if err := runProjectSetup(context.Background(), nil); err == nil {
		t.Error("expected an error when devx.yaml doesn't exist")
	}
}

func TestRunProjectSetup_NothingToDo_JSON(t *testing.T) {
	writeManifest(t, validManifest)

	var setupErr error
	out := captureStdout(t, func() {
		setupErr = runProjectSetup(context.Background(), []string{"--json"})
	})
	if setupErr != nil {
		t.Fatalf("runProjectSetup failed: %v", setupErr)
	}
	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &results); err != nil {
		t.Fatalf("expected valid JSON array, got %q: %v", out, err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for manifest with no tools/setup, got %d", len(results))
	}
}

func TestRunProjectSetup_HumanReadableSuccessPath(t *testing.T) {
	// 'go' is always available wherever these tests run.
	writeManifest(t, `version: 1
project:
  name: my-app
api:
  run: "echo hi"
tools:
  - name: go
    check: go version
setup:
  - name: echo-hello
    run: echo hello
`)

	var setupErr error
	out := captureStdout(t, func() {
		setupErr = runProjectSetup(context.Background(), nil)
	})
	if setupErr != nil {
		t.Fatalf("runProjectSetup failed: %v\noutput:\n%s", setupErr, out)
	}
	for _, want := range []string{"Checking tools...", "Running setup steps...", "go", "echo-hello", "Dev environment is ready."} {
		if !strings.Contains(out, want) {
			t.Errorf("expected human-readable output to contain %q, got:\n%s", want, out)
		}
	}
}
func TestRunProjectSetup_MissingToolReturnsError(t *testing.T) {
	writeManifest(t, `version: 1
project:
  name: my-app
api:
  run: "echo hi"
tools:
  - name: definitely-missing-tool
    check: "exit 1"
`)
	_ = captureStdout(t, func() {
		if err := runProjectSetup(context.Background(), nil); err == nil {
			t.Error("expected an error when a required tool is missing and --fix wasn't passed")
		}
	})
}
