package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStackFromDevcontainerFeatures(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"dotnet feature", `{"features": {"ghcr.io/devcontainers/features/dotnet:2": {}}}`, "dotnet"},
		{"node feature", `{"features": {"ghcr.io/devcontainers/features/node:1": {}}}`, "node"},
		{"go feature", `{"features": {"ghcr.io/devcontainers/features/go:1": {}}}`, "go"},
		{"python feature", `{"features": {"ghcr.io/devcontainers/features/python:1": {}}}`, "python"},
		{"image fallback dotnet", `{"image": "mcr.microsoft.com/dotnet/sdk:8.0"}`, "dotnet"},
		{"no match", `{"image": "ubuntu:22.04"}`, ""},
		{"invalid json", `not json`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stackFromDevcontainerFeatures([]byte(tc.raw)); got != tc.want {
				t.Errorf("stackFromDevcontainerFeatures(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestGlobExists(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	if globExists("*.csproj") {
		t.Error("expected no .csproj files in an empty dir")
	}
	if err := os.WriteFile(filepath.Join(dir, "app.csproj"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if !globExists("*.csproj") {
		t.Error("expected globExists to find app.csproj")
	}
}

func TestReadJSONC_StripsComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "devcontainer.json")
	content := "{\n  // a comment\n  \"name\": \"test\"\n}\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	data, err := readJSONC(path)
	if err != nil {
		t.Fatalf("readJSONC failed: %v", err)
	}
	if string(data) == content {
		t.Error("expected comments to be stripped")
	}
}

func TestReadJSONC_MissingFile(t *testing.T) {
	if _, err := readJSONC(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestDetectStack_FromGoMod(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := detectStack(); got != "go" {
		t.Errorf("detectStack() = %q, want %q", got, "go")
	}
}

func TestDetectStack_FromPackageJSON(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := detectStack(); got != "node" {
		t.Errorf("detectStack() = %q, want %q", got, "node")
	}
}

func TestDetectStack_Other(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	if got := detectStack(); got != "other" {
		t.Errorf("detectStack() = %q, want %q", got, "other")
	}
}

func TestGenerateCIWorkflow_KnownStacks(t *testing.T) {
	for _, stack := range []string{"dotnet", "node", "go", "python", "other", "unknown-stack"} {
		got := generateCIWorkflow(stack)
		if got == "" {
			t.Errorf("generateCIWorkflow(%q) returned empty output", stack)
		}
	}
}
