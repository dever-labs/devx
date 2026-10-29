package main

import (
	"strings"
	"testing"
)

// devcontainer CLI is never installed in this sandbox, so these exercise
// the error paths deterministically.

func TestDevcontainerBuild_NoCLIErrors(t *testing.T) {
	if err := devcontainerBuild(".", false); err == nil {
		t.Fatal("expected error when devcontainer CLI is not installed")
	}
}

func TestRunDevcontainerCLI_NoCLIErrors(t *testing.T) {
	if err := runDevcontainerCLI("up", "--workspace-folder", "."); err == nil {
		t.Fatal("expected error when devcontainer CLI is not installed")
	}
}

func TestRunDevLogs_NoContainerRunningErrors(t *testing.T) {
	chdirTemp(t)
	err := runDevLogs(nil)
	if err == nil {
		t.Fatal("expected error when no devcontainer is running")
	}
	if !strings.Contains(err.Error(), "no devcontainer running") {
		t.Errorf("error = %q, want it to mention 'no devcontainer running'", err)
	}
}

func TestRunDevLogs_FollowFlagParses(t *testing.T) {
	chdirTemp(t)
	err := runDevLogs([]string{"--follow"})
	if err == nil {
		t.Fatal("expected error when no devcontainer is running")
	}
}

func TestRunDevShell_NoCLIErrors(t *testing.T) {
	chdirTemp(t)
	if err := runDevShell(); err == nil {
		t.Fatal("expected error when devcontainer CLI is not installed")
	}
}
