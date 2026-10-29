package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCI_PrintsWorkflowToStdoutByDefault(t *testing.T) {
	chdirTemp(t)
	var err error
	out := captureStdout(t, func() {
		err = runCI(context.Background(), []string{"--stack", "go"})
	})
	if err != nil {
		t.Fatalf("runCI failed: %v", err)
	}
	if !strings.Contains(out, "go test") && !strings.Contains(out, "go build") {
		t.Errorf("expected a Go CI workflow in stdout, got:\n%s", out)
	}
	if _, statErr := os.Stat(".github/workflows/ci.yml"); statErr == nil {
		t.Error("expected no file to be written without --write")
	}
}

func TestRunCI_WriteCreatesWorkflowFile(t *testing.T) {
	dir := chdirTemp(t)
	_ = captureStdout(t, func() {
		if err := runCI(context.Background(), []string{"--stack", "node", "--write"}); err != nil {
			t.Fatalf("runCI --write failed: %v", err)
		}
	})
	path := filepath.Join(dir, ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected ci.yml to be written: %v", err)
	}
	if !strings.Contains(string(data), "npm") {
		t.Errorf("expected a Node CI workflow, got:\n%s", data)
	}
}
