package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dever-labs/devx/internal/task"
)

func TestRunLogs_MissingManifestErrors(t *testing.T) {
	chdirTemp(t)
	if err := runLogs(context.Background(), nil); err == nil {
		t.Error("expected an error when devx.yaml doesn't exist")
	}
}

func TestRunLogs_UnknownTaskErrors(t *testing.T) {
	dir := chdirTemp(t)
	manifest := "version: 1\n\napi:\n  run: echo api\n"
	if err := os.WriteFile(filepath.Join(dir, manifestFile), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	err := runLogs(context.Background(), []string{"nope"})
	if err == nil || !strings.Contains(err.Error(), "unknown task") {
		t.Errorf("runLogs(nope) = %v, want 'unknown task' error", err)
	}
}

func TestRunLogs_NotRunningTaskErrors(t *testing.T) {
	dir := chdirTemp(t)
	manifest := "version: 1\n\napi:\n  run: echo api\n"
	if err := os.WriteFile(filepath.Join(dir, manifestFile), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	err := runLogs(context.Background(), []string{"api"})
	if err == nil || !strings.Contains(err.Error(), "isn't currently running") {
		t.Errorf("runLogs(api) for a not-running task = %v, want 'isn't currently running' error", err)
	}
}

func TestRunLogs_NoTasksRunningPrintsMessage(t *testing.T) {
	dir := chdirTemp(t)
	manifest := "version: 1\n\napi:\n  run: echo api\n"
	if err := os.WriteFile(filepath.Join(dir, manifestFile), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	var logsErr error
	out := captureStdout(t, func() {
		logsErr = runLogs(context.Background(), nil)
	})
	if logsErr != nil {
		t.Fatalf("runLogs failed: %v", logsErr)
	}
	if !strings.Contains(out, "No tasks currently running") {
		t.Errorf("expected the no-tasks-running message, got:\n%s", out)
	}
}

func TestRunLogs_TailsSpecificRunningTask(t *testing.T) {
	dir := chdirTemp(t)
	manifest := "version: 1\n\napi:\n  run: echo api\n"
	if err := os.WriteFile(filepath.Join(dir, manifestFile), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, devxDir), 0755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, devxDir, "api.log")
	if err := os.WriteFile(logPath, []byte("api started\n"), 0644); err != nil {
		t.Fatal(err)
	}

	state, err := task.LoadState(stateFilePath())
	if err != nil {
		t.Fatal(err)
	}
	state.Record("api", task.StateEntry{LogPath: logPath, PID: os.Getpid()})
	if err := state.Save(stateFilePath()); err != nil {
		t.Fatal(err)
	}

	var logsErr error
	out := captureStdout(t, func() {
		logsErr = runLogs(context.Background(), []string{"api"})
	})
	if logsErr != nil {
		t.Fatalf("runLogs(api) failed: %v", logsErr)
	}
	if !strings.Contains(out, "[api] api started") {
		t.Errorf("expected the running task's log output, got:\n%s", out)
	}
}

func TestTailAll_NonFollowPrintsAllTasksSorted(t *testing.T) {
	dir := chdirTemp(t)
	logA := filepath.Join(dir, "a.log")
	logB := filepath.Join(dir, "b.log")
	if err := os.WriteFile(logA, []byte("from a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logB, []byte("from b\n"), 0644); err != nil {
		t.Fatal(err)
	}

	entries := map[string]task.StateEntry{
		"a": {LogPath: logA},
		"b": {LogPath: logB},
	}
	out := captureStdout(t, func() {
		if err := tailAll(context.Background(), entries, false); err != nil {
			t.Fatalf("tailAll failed: %v", err)
		}
	})
	if !strings.Contains(out, "[a] from a") || !strings.Contains(out, "[b] from b") {
		t.Errorf("tailAll output missing expected lines, got:\n%s", out)
	}
}
