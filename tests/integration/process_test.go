//go:build integration && !windows

package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const processManifest = `version: 1
project:
  name: proc-demo
  defaultProfile: local
profiles:
  local:
    services:
      worker:
        kind: process
        run: "echo started; sleep 300"
`

// Process-only profiles must work without any container runtime.
func TestProcess_UpStatusLogsDown(t *testing.T) {
	dir := withManifest(t, processManifest)
	t.Cleanup(func() { run(t, dir, "down") })

	if _, stderr, code := run(t, dir, "up"); code != 0 {
		t.Fatalf("up failed (%d): %s", code, stderr)
	}

	out, _, code := run(t, dir, "status", "--json")
	if code != 0 {
		t.Fatalf("status failed")
	}
	var rows []struct {
		Name  string `json:"name"`
		Kind  string `json:"kind"`
		State string `json:"state"`
		PID   int    `json:"pid"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("bad status json: %v\n%s", err, out)
	}
	if len(rows) != 1 || rows[0].Name != "worker" || rows[0].Kind != "process" || rows[0].State != "running" || rows[0].PID == 0 {
		t.Fatalf("unexpected status: %+v", rows)
	}

	if out, _, _ := run(t, dir, "logs", "worker"); !strings.Contains(out, "started") {
		t.Errorf("logs missing output: %q", out)
	}

	if _, _, code := run(t, dir, "down"); code != 0 {
		t.Fatalf("down failed")
	}
	out, _, _ = run(t, dir, "status", "--json")
	if strings.Contains(out, `"running"`) {
		t.Errorf("process still running after down: %s", out)
	}
}

func TestProcess_ReadinessFailureFailsFast(t *testing.T) {
	dir := withManifest(t, `version: 1
project:
  name: proc-demo
  defaultProfile: local
profiles:
  local:
    services:
      crash:
        kind: process
        run: "exit 3"
        ports: ["45999"]
`)
	t.Cleanup(func() { run(t, dir, "down") })
	if _, _, code := run(t, dir, "up"); code == 0 {
		t.Fatal("expected up to fail when the process exits before becoming ready")
	}
}

func TestSetup_DetectsToolVersionFromGoMod(t *testing.T) {
	dir := withManifest(t, `version: 1
project:
  name: app
  defaultProfile: local
profiles:
  local:
    services:
      api:
        image: nginx:alpine
tools:
  - name: go
    check: "echo go1.0.0"
`)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n\ngo 1.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, code := run(t, dir, "setup", "--json")
	if code != 0 {
		t.Fatalf("setup failed: %s", out)
	}
	var res []map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if res[0]["source"] != "go.mod" {
		t.Errorf("expected source go.mod, got %v", res[0])
	}
}

func TestSetup_EnvFiles(t *testing.T) {
	dir := withManifest(t, `version: 1
project:
  name: app
  defaultProfile: local
profiles:
  local:
    services:
      api:
        image: nginx:alpine
envFiles:
  - file: .env.local
    values:
      TOKEN: { from: env, name: DEVX_IT_TOKEN }
`)
	if _, _, code := run(t, dir, "setup", "--json", "--non-interactive"); code == 0 {
		t.Fatal("expected failure when a value is missing")
	}
	t.Setenv("DEVX_IT_TOKEN", "abc123")
	out, _, code := run(t, dir, "setup", "--json", "--non-interactive")
	if code != 0 {
		t.Fatalf("setup failed: %s", out)
	}
	if strings.Contains(out, "abc123") {
		t.Error("secret value leaked into --json output")
	}
	b, err := os.ReadFile(filepath.Join(dir, ".env.local"))
	if err != nil || strings.TrimSpace(string(b)) != "TOKEN=abc123" {
		t.Fatalf("env file: %q %v", b, err)
	}
}
