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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestDevxDoctor(t *testing.T) {
	bin := devxBin(t)
	out, err := exec.Command(bin, "doctor").CombinedOutput()
	if err != nil {
		t.Logf("doctor output:\n%s", out)
		t.Fatalf("devx doctor failed: %v", err)
	}
}

func TestDevxUp_Basic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	bin := devxBin(t)
	dir := exampleDir("basic")

	// Bring up the environment
	up := exec.Command(bin, "up", "--profile", "ci")
	up.Dir = dir
	up.Stdout = os.Stdout
	up.Stderr = os.Stderr
	if err := up.Run(); err != nil {
		t.Fatalf("devx up failed: %v", err)
	}

	// Always tear down after the test
	t.Cleanup(func() {
		down := exec.Command(bin, "down", "--volumes")
		down.Dir = dir
		down.Stdout = os.Stdout
		down.Stderr = os.Stderr
		if err := down.Run(); err != nil {
			t.Logf("warning: devx down failed: %v", err)
		}
	})

	// Check status reports running services
	time.Sleep(2 * time.Second)
	status := exec.Command(bin, "status")
	status.Dir = dir
	out, err := status.CombinedOutput()
	if err != nil {
		t.Fatalf("devx status failed: %v\n%s", err, out)
	}
	t.Logf("status output:\n%s", out)
}

func TestDevxRenderCompose_Basic(t *testing.T) {
	bin := devxBin(t)
	dir := exampleDir("basic")

	out, err := exec.Command(bin, "render", "compose").
		Output()
	if err != nil {
		t.Fatalf("devx render compose failed: %v", err)
	}

	_ = dir
	rendered := string(out)
	for _, want := range []string{"services:", "networks:", "devx_default"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered compose missing %q", want)
		}
	}
}
