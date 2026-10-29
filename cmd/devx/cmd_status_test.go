package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunStatus_JSON_NoRunningTasks(t *testing.T) {
	dir := chdirTemp(t)
	manifest := "version: 1\n\napi:\n  run: echo api\n\nweb:\n  run: echo web\n"
	if err := os.WriteFile(filepath.Join(dir, manifestFile), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	var statusErr error
	out := captureStdout(t, func() {
		statusErr = runStatus(context.Background(), []string{"--json"})
	})
	if statusErr != nil {
		t.Fatalf("runStatus failed: %v", statusErr)
	}

	var rows []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("runStatus --json produced invalid JSON: %v\noutput: %s", err, out)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 task rows, got %d: %+v", len(rows), rows)
	}
	for _, r := range rows {
		if r.Status != "not running" {
			t.Errorf("expected task %q to be 'not running' with no state file, got %q", r.Name, r.Status)
		}
	}
}

func TestRunStatus_JSON_NoManifest(t *testing.T) {
	chdirTemp(t)
	if err := runStatus(context.Background(), []string{"--json"}); err == nil {
		t.Fatal("expected an error when devx.yaml does not exist")
	}
}

func TestRunStatus_Human_NoRunningTasks(t *testing.T) {
	dir := chdirTemp(t)
	manifest := "version: 1\n\napi:\n  run: echo api\n"
	if err := os.WriteFile(filepath.Join(dir, manifestFile), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	var statusErr error
	out := captureStdout(t, func() {
		statusErr = runStatus(context.Background(), nil)
	})
	if statusErr != nil {
		t.Fatalf("runStatus failed: %v", statusErr)
	}
	if !strings.Contains(out, "api") || !strings.Contains(out, "not running") {
		t.Errorf("expected human output to mention the task and its status, got:\n%s", out)
	}
}

func TestDevcontainerState_NotConfigured(t *testing.T) {
	chdirTemp(t)
	if got := devcontainerState(); got != "not configured" {
		t.Errorf("devcontainerState() = %q, want %q", got, "not configured")
	}
}

func TestAIStatusLine_NotConfigured(t *testing.T) {
	chdirTemp(t)
	if got := aiStatusLine(); !strings.Contains(got, "not configured") {
		t.Errorf("aiStatusLine() = %q, want it to mention 'not configured'", got)
	}
}
