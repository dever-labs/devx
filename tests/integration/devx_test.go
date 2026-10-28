//go:build integration

// Package integration contains end-to-end tests for devx.
// These tests require Docker to be running and the devx binary on PATH.
//
// Run with:
//
//	go test ./tests/integration/... -tags integration -v
//	go test ./tests/integration/... -tags integration -v -run TestDevxUp
package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// devxBin returns the path to the devx binary under test.
// Prefers DEVX_BIN env var, falls back to "devx" on PATH.
func devxBin(t *testing.T) string {
	t.Helper()
	if bin := os.Getenv("DEVX_BIN"); bin != "" {
		return bin
	}
	bin, err := exec.LookPath("devx")
	if err != nil {
		t.Skip("devx binary not found on PATH; set DEVX_BIN or install devx")
	}
	return bin
}

// exampleDir returns the path to a named example directory.
func exampleDir(name string) string {
	return filepath.Join("..", "..", "examples", name)
}

// writeTaskManifest writes a flat Task-model devx.yaml to a fresh temp
// dir and returns the dir.
func writeTaskManifest(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "devx.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDevxVersion(t *testing.T) {
	bin := devxBin(t)
	out, err := exec.Command(bin, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("devx version failed: %v\n%s", err, out)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(out)), "devx") {
		t.Errorf("unexpected version output: %q", string(out))
	}
}

// TestDevxDoctor runs `devx doctor` and only treats it as a hard
// failure if something unrelated to the optional podman runtime is
// wrong — most dev/CI boxes have Docker only, and doctor (by design)
// checks both docker and podman and reports whichever is absent.
func TestDevxDoctor(t *testing.T) {
	bin := devxBin(t)
	cmd := exec.Command(bin, "doctor", "--json")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		var checks []map[string]any
		if jsonErr := json.Unmarshal([]byte(stdout.String()), &checks); jsonErr != nil {
			t.Fatalf("devx doctor failed and stdout isn't valid JSON: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
		}
		for _, c := range checks {
			name, _ := c["name"].(string)
			status, _ := c["status"].(string)
			if status == "FAIL" && name != "Runtime: podman" {
				t.Fatalf("devx doctor reported an unexpected failure: %v", c)
			}
		}
	}
}

// TestDevxUp_Basic exercises the full real-world example (db, cache,
// migrate, api, worker — built/started via plain docker run/build in
// run:) end to end: up -d, status, down. Requires Docker.
func TestDevxUp_Basic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found on PATH")
	}

	bin := devxBin(t)
	dir := exampleDir("basic")

	up := exec.Command(bin, "up", "-d")
	up.Dir = dir
	up.Stdout = os.Stdout
	up.Stderr = os.Stderr
	if err := up.Run(); err != nil {
		t.Fatalf("devx up -d failed: %v", err)
	}

	t.Cleanup(func() {
		down := exec.Command(bin, "down")
		down.Dir = dir
		down.Stdout = os.Stdout
		down.Stderr = os.Stderr
		if err := down.Run(); err != nil {
			t.Logf("warning: devx down failed: %v", err)
		}
	})

	time.Sleep(2 * time.Second)
	status := exec.Command(bin, "status")
	status.Dir = dir
	out, err := status.CombinedOutput()
	if err != nil {
		t.Fatalf("devx status failed: %v\n%s", err, out)
	}
	t.Logf("status output:\n%s", out)
}

// TestDevxDown_StopsBackgroundProcess proves `devx down` actually
// terminates a detached plain-process task (not a container) by PID —
// the exact behavior that was found broken (and fixed) in review: the
// scheduler must record a real PID in .devx/tasks.json, and `devx down`
// must signal that process group, not just drop the state entry.
// No Docker required.
func TestDevxDown_StopsBackgroundProcess(t *testing.T) {
	bin := devxBin(t)
	dir := writeTaskManifest(t, `version: 1
project:
  name: down-stop-test
server:
  run: "sleep 300"
  detached: true
  ready:
    cmd: "true"
`)

	up := exec.Command(bin, "up", "-d")
	up.Dir = dir
	out, err := up.CombinedOutput()
	if err != nil {
		t.Fatalf("devx up -d failed: %v\n%s", err, out)
	}

	pid := pidFromState(t, dir, "server")
	if syscall.Kill(pid, 0) != nil {
		t.Fatalf("expected task %q's recorded pid %d to be alive right after up -d", "server", pid)
	}

	down := exec.Command(bin, "down")
	down.Dir = dir
	if out, err := down.CombinedOutput(); err != nil {
		t.Fatalf("devx down failed: %v\n%s", err, out)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return // gone, as expected
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("expected pid %d to be gone after devx down, but it's still running", pid)
}

// pidFromState reads .devx/tasks.json (internal/task.StateStore's
// on-disk format) and returns the recorded PID for the given task name.
func pidFromState(t *testing.T, dir, name string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".devx", "tasks.json"))
	if err != nil {
		t.Fatalf("read tasks.json: %v", err)
	}
	var state struct {
		Entries map[string]struct {
			PID int `json:"pid"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("parse tasks.json: %v\n%s", err, data)
	}
	entry, ok := state.Entries[name]
	if !ok {
		t.Fatalf("no state entry for task %q in tasks.json: %s", name, data)
	}
	if entry.PID <= 0 {
		t.Fatalf("expected a positive pid for task %q, got %d", name, entry.PID)
	}
	return entry.PID
}

// TestDevxTasksJSON exercises `devx tasks --json` against a real
// multi-task graph and checks the resolved shape round-trips.
func TestDevxTasksJSON(t *testing.T) {
	bin := devxBin(t)
	dir := writeTaskManifest(t, `version: 1
project:
  name: tasks-json-test
build:
  run: "echo building"
test:
  deps: [build]
  run: "echo testing"
`)

	cmd := exec.Command(bin, "tasks", "--json")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("devx tasks --json failed: %v\n%s", err, out)
	}

	var tasks []map[string]any
	if err := json.Unmarshal(out, &tasks); err != nil {
		t.Fatalf("expected valid JSON, got %v\n%s", err, out)
	}
	names := map[string]bool{}
	for _, task := range tasks {
		names[task["name"].(string)] = true
	}
	if !names["build"] || !names["test"] {
		t.Errorf("expected both build and test in task list, got %v", names)
	}
}

// TestDevxRunShorthand proves `devx <name>` is equivalent to
// `devx run <name>` for a task name that isn't a built-in command.
func TestDevxRunShorthand(t *testing.T) {
	bin := devxBin(t)
	dir := writeTaskManifest(t, `version: 1
project:
  name: shorthand-test
greet:
  run: "echo hello-from-greet"
`)

	cmd := exec.Command(bin, "greet")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("devx greet (shorthand) failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "[greet]") {
		t.Errorf("expected scheduler status output to mention the task, got %q", out)
	}

	// Task stdout/stderr for non-interactive runs is captured to
	// .devx/logs/<name>.log, not echoed to the CLI's own stdout.
	logData, err := os.ReadFile(filepath.Join(dir, ".devx", "logs", "greet.log"))
	if err != nil {
		t.Fatalf("read greet.log: %v", err)
	}
	if !strings.Contains(string(logData), "hello-from-greet") {
		t.Errorf("expected greet.log to contain task's echo, got %q", logData)
	}
}

// TestDevxRemovedCommands_ReturnDocumentedError asserts that the
// Profile/Compose-era render/lock/providers/export subcommands are
// permanently stubbed out with an explanatory error rather than silently
// doing nothing or producing stale compose/k8s output.
func TestDevxRemovedCommands_ReturnDocumentedError(t *testing.T) {
	bin := devxBin(t)
	dir := exampleDir("basic")

	for _, cmd := range []string{"render", "lock", "providers", "export"} {
		t.Run(cmd, func(t *testing.T) {
			c := exec.Command(bin, cmd)
			c.Dir = dir
			out, err := c.CombinedOutput()
			if err == nil {
				t.Fatalf("expected devx %s to fail, got exit 0: %s", cmd, out)
			}
			if !strings.Contains(string(out), "not supported in the task-based devx.yaml model") {
				t.Errorf("devx %s: expected documented removal error, got %q", cmd, out)
			}
		})
	}
}
