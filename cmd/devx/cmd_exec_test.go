package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunExec_NoArgsErrors(t *testing.T) {
	if err := runExec(context.Background(), nil); err == nil {
		t.Error("expected an error when no args are given")
	}
}

func TestRunExec_NoSeparatorErrors(t *testing.T) {
	if err := runExec(context.Background(), []string{"api"}); err == nil {
		t.Error("expected an error when there's no -- separator")
	}
}

func TestRunExec_NoCommandAfterSeparatorErrors(t *testing.T) {
	if err := runExec(context.Background(), []string{"api", "--"}); err == nil {
		t.Error("expected an error when there's no command after --")
	}
}

func TestRunExec_MissingManifestErrors(t *testing.T) {
	chdirTemp(t)
	if err := runExec(context.Background(), []string{"api", "--", "echo", "hi"}); err == nil {
		t.Error("expected an error when devx.yaml doesn't exist")
	}
}

func TestRunExec_UnknownTaskErrors(t *testing.T) {
	dir := chdirTemp(t)
	manifest := "version: 1\n\napi:\n  run: echo api\n"
	if err := os.WriteFile(filepath.Join(dir, manifestFile), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	err := runExec(context.Background(), []string{"unknown-task", "--", "echo", "hi"})
	if err == nil || !strings.Contains(err.Error(), "unknown task") {
		t.Errorf("runExec with unknown task = %v, want 'unknown task' error", err)
	}
}

func TestContainerStatus_NonexistentContainerReturnsExited(t *testing.T) {
	status, err := containerStatus(context.Background(), "devx-nonexistent-container-xyz")
	if err != nil {
		t.Fatalf("containerStatus returned an error: %v", err)
	}
	if status != "exited" {
		t.Errorf("containerStatus for a nonexistent container = %q, want %q", status, "exited")
	}
}
