package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDevStatus_NoDevcontainerErrors(t *testing.T) {
	chdirTemp(t)
	if err := runDevStatus(); err == nil {
		t.Error("expected an error when no devcontainer.json exists")
	}
}

func TestRunDevStatus_PrintsConfigDetails(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.MkdirAll(filepath.Join(dir, ".devcontainer"), 0755); err != nil {
		t.Fatal(err)
	}
	content := `{
		"name": "My Dev Container",
		"image": "ghcr.io/dever-labs/devcontainers/go-dev:latest",
		"features": {"ghcr.io/devcontainers/features/go:1": {}},
		"forwardPorts": [8080, 5432],
		"customizations": {"vscode": {"extensions": ["golang.go"]}}
	}`
	path := filepath.Join(dir, ".devcontainer", "devcontainer.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := runDevStatus(); err != nil {
			t.Fatalf("runDevStatus failed: %v", err)
		}
	})

	for _, want := range []string{"My Dev Container", "go-dev:latest", "go:1", "8080, 5432", "golang.go"} {
		if !strings.Contains(out, want) {
			t.Errorf("runDevStatus output missing %q, got:\n%s", want, out)
		}
	}
}
