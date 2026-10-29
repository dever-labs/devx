package main

import (
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
