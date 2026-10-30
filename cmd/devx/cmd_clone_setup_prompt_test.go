package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// newGitRepoWithSetupStep creates a local git repo (so it can be cloned via
// a plain file-path URL) containing a devx.yaml whose setup step creates a
// marker file. Returns the repo path and the marker file path setup would
// produce once run inside a clone of it.
func newGitRepoWithSetupStep(t *testing.T) (repoDir, markerName string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoDir = t.TempDir()
	markerName = "setup-ran.marker"
	manifest := `version: 1
project:
  name: cloned-app
setup:
  - name: mark
    run: "touch ` + markerName + `"
api:
  run: "echo hi"
`
	if err := os.WriteFile(filepath.Join(repoDir, "devx.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("add", "-A")
	run("commit", "-q", "-m", "initial")
	return repoDir, markerName
}

// TestRunClone_SetupNotRunWithoutConfirmationOrYes verifies the HIGH-severity
// fix: devx.yaml's setup steps (which can run arbitrary shell commands) must
// never execute silently right after a clone. In this non-interactive test
// environment (stdin isn't a terminal), neither a bare `devx clone` nor one
// without --yes should run setup.
func TestRunClone_SetupNotRunWithoutConfirmationOrYes(t *testing.T) {
	repoDir, markerName := newGitRepoWithSetupStep(t)
	destParent := chdirTemp(t)

	if err := runClone(context.Background(), []string{repoDir, "cloned"}); err != nil {
		t.Fatalf("runClone failed: %v", err)
	}

	markerPath := filepath.Join(destParent, "cloned", markerName)
	if _, err := os.Stat(markerPath); err == nil {
		t.Fatal("setup ran without confirmation or --yes; expected it to be skipped in a non-interactive environment")
	}
}

// TestRunClone_SetupRunsWithYes verifies --yes still runs setup, for
// automation use cases that want the old always-run behavior explicitly.
func TestRunClone_SetupRunsWithYes(t *testing.T) {
	repoDir, markerName := newGitRepoWithSetupStep(t)
	destParent := chdirTemp(t)

	if err := runClone(context.Background(), []string{"--yes", repoDir, "cloned"}); err != nil {
		t.Fatalf("runClone failed: %v", err)
	}

	markerPath := filepath.Join(destParent, "cloned", markerName)
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("expected --yes to run setup and create %s, got: %v", markerPath, err)
	}
}

// TestRunClone_NoSetupFlagSkipsSetup keeps the pre-existing --no-setup
// escape hatch working after the confirmation-gating change.
func TestRunClone_NoSetupFlagSkipsSetup(t *testing.T) {
	repoDir, markerName := newGitRepoWithSetupStep(t)
	destParent := chdirTemp(t)

	if err := runClone(context.Background(), []string{"--no-setup", repoDir, "cloned"}); err != nil {
		t.Fatalf("runClone failed: %v", err)
	}

	markerPath := filepath.Join(destParent, "cloned", markerName)
	if _, err := os.Stat(markerPath); err == nil {
		t.Fatal("--no-setup should have prevented setup from running")
	}
}
