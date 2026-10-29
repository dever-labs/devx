package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunEnv_NoVarsPrintsMessage(t *testing.T) {
	chdirTemp(t)
	out := captureStdout(t, func() {
		if err := runEnv(context.Background(), nil); err != nil {
			t.Fatalf("runEnv failed: %v", err)
		}
	})
	if !strings.Contains(out, "No environment variables defined") {
		t.Errorf("expected the no-vars message, got:\n%s", out)
	}
}

func TestRunEnv_ListsDotEnvVars(t *testing.T) {
	dir := chdirTemp(t)
	content := "API_KEY=supersecretvalue\nPORT=8080\n# a comment\n\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := runEnv(context.Background(), nil); err != nil {
			t.Fatalf("runEnv failed: %v", err)
		}
	})
	if !strings.Contains(out, "PORT") || !strings.Contains(out, "8080") {
		t.Errorf("expected PORT=8080 to be listed, got:\n%s", out)
	}
	if !strings.Contains(out, "API_KEY") {
		t.Errorf("expected API_KEY to be listed, got:\n%s", out)
	}
	if strings.Contains(out, "supersecretvalue") {
		t.Errorf("expected API_KEY's value to be masked by default, got:\n%s", out)
	}
}

func TestRunEnv_ShowRevealsSecretValues(t *testing.T) {
	dir := chdirTemp(t)
	content := "API_KEY=supersecretvalue\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := runEnv(context.Background(), []string{"--show"}); err != nil {
			t.Fatalf("runEnv --show failed: %v", err)
		}
	})
	if !strings.Contains(out, "supersecretvalue") {
		t.Errorf("expected --show to reveal the value, got:\n%s", out)
	}
}

func TestRunEnv_ReportsMissingValues(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.MkdirAll(filepath.Join(dir, ".devcontainer"), 0755); err != nil {
		t.Fatal(err)
	}
	dc := `{"remoteEnv": {"DATABASE_URL": ""}}`
	if err := os.WriteFile(filepath.Join(dir, ".devcontainer", "devcontainer.json"), []byte(dc), 0644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := runEnv(context.Background(), nil); err != nil {
			t.Fatalf("runEnv failed: %v", err)
		}
	})
	if !strings.Contains(out, "not set") {
		t.Errorf("expected a 'not set' warning for an empty remoteEnv value, got:\n%s", out)
	}
}
