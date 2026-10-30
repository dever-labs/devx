package main

import (
	"strings"
	"testing"
)

func TestRequireDevcontainerCLI_MissingReturnsHelpfulError(t *testing.T) {
	// The devcontainer CLI isn't installed in this test environment, so
	// this exercises the "not found" path deterministically.
	err := requireDevcontainerCLI()
	if err == nil {
		t.Skip("devcontainer CLI is installed in this environment; nothing to verify")
	}
	if !strings.Contains(err.Error(), "devcontainer CLI not found") {
		t.Errorf("requireDevcontainerCLI() error = %v, want mention of 'devcontainer CLI not found'", err)
	}
}

func TestDevcontainerID_NoneRunningReturnsError(t *testing.T) {
	_, err := devcontainerID()
	if err == nil {
		t.Skip("a devcontainer for this workspace is apparently running; nothing to verify")
	}
	// Some CI runners (notably windows-latest) ship a 'docker' CLI with no
	// reachable daemon — the docker ps call then fails with its own
	// wrapped error instead of reaching the "no devcontainer running"
	// branch. Both are valid shapes of "couldn't find a running
	// devcontainer"; assert on either rather than coupling to a specific
	// host's docker availability.
	if !strings.Contains(err.Error(), "no devcontainer running") && !strings.Contains(err.Error(), "docker") {
		t.Errorf("devcontainerID() error = %v, want mention of 'no devcontainer running' or a docker-related failure", err)
	}
}

func TestPrintDevUsage_MentionsSubcommands(t *testing.T) {
	out := captureStdout(t, printDevUsage)
	for _, want := range []string{"devx dev build", "devx dev up", "devx dev shell", "devx dev logs"} {
		if !strings.Contains(out, want) {
			t.Errorf("printDevUsage() output missing %q, got:\n%s", want, out)
		}
	}
}
