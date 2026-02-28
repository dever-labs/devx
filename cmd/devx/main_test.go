package main

import (
	"os"
	"path/filepath"
	"testing"
)

const validManifest = `version: 1
project:
  name: my-app
  defaultProfile: local
profiles:
  local:
    runtime: compose
    services:
      api:
        image: nginx:alpine
  ci:
    runtime: compose
    services:
      api:
        image: nginx:alpine
`

// chdirTemp writes a devx.yaml to a temp dir and chdirs into it.
// Returns a cleanup func that restores the original working directory.
func chdirTemp(t *testing.T, content string) func() {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "devx.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return func() {
		if err := os.Chdir(orig); err != nil {
			t.Logf("warning: could not restore working directory: %v", err)
		}
	}
}

func TestLoadProfile_DefaultProfile(t *testing.T) {
	defer chdirTemp(t, validManifest)()

	_, profName, _, err := loadProfile("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if profName != "local" {
		t.Fatalf("expected profile 'local', got %q", profName)
	}
}

func TestLoadProfile_ExplicitProfile(t *testing.T) {
	defer chdirTemp(t, validManifest)()

	_, profName, _, err := loadProfile("ci")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if profName != "ci" {
		t.Fatalf("expected profile 'ci', got %q", profName)
	}
}

func TestLoadProfile_NoDefaultProfile(t *testing.T) {
	defer chdirTemp(t, `version: 1
project:
  name: my-app
  defaultProfile: ""
profiles:
  local:
    runtime: compose
    services:
      api:
        image: nginx:alpine
`)()

	_, _, _, err := loadProfile("")
	if err == nil {
		t.Fatal("expected error when no defaultProfile is set")
	}
}

func TestLoadProfile_MissingProfile(t *testing.T) {
	defer chdirTemp(t, validManifest)()

	_, _, _, err := loadProfile("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
}

func TestLoadProfile_NoManifest(t *testing.T) {
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)

	_, _, _, err = loadProfile("")
	if err == nil {
		t.Fatal("expected error when devx.yaml is missing")
	}
}
