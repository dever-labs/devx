package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunAIModel_NoArgsPrintsUsage(t *testing.T) {
	var err error
	out := captureStdout(t, func() {
		err = runAIModel(nil)
	})
	if err != nil {
		t.Fatalf("runAIModel() failed: %v", err)
	}
	if !strings.Contains(out, "devx ai model list") {
		t.Errorf("expected usage output, got:\n%s", out)
	}
}

func TestRunAIModel_PullNoNameErrors(t *testing.T) {
	chdirTemp(t)
	if err := runAIModel([]string{"pull"}); err == nil {
		t.Error("expected an error when no model name is given to pull")
	}
}

func TestRunAIModel_RemoveNoNameErrors(t *testing.T) {
	chdirTemp(t)
	if err := runAIModel([]string{"remove"}); err == nil {
		t.Error("expected an error when no model name is given to remove")
	}
}

func TestRunAIModel_UnknownSubcommandErrors(t *testing.T) {
	chdirTemp(t)
	if err := runAIModel([]string{"bogus"}); err == nil {
		t.Error("expected an error for an unknown model subcommand")
	}
}

func TestRunAIModelList_HandlesBackendPresenceGracefully(t *testing.T) {
	chdirTemp(t)
	var err error
	_ = captureStdout(t, func() {
		err = runAIModel([]string{"list"})
	})
	// This sandbox sometimes has a real Ollama server reachable via
	// host.docker.internal, so we can't assert a single deterministic
	// outcome — just confirm it fails gracefully (no backend, or backend
	// detected but the `ollama` CLI itself isn't installed) rather than
	// panicking, or succeeds if the CLI happens to be present too.
	if err != nil &&
		!strings.Contains(err.Error(), "no AI backend running") &&
		!strings.Contains(err.Error(), "executable file not found") {
		t.Errorf("unexpected error from runAIModel(list): %v", err)
	}
}

func TestRunAIChat_NoConfigErrors(t *testing.T) {
	chdirTemp(t)
	if err := runAIChat([]string{"hello"}); err == nil {
		t.Error("expected an error when no AI is configured")
	} else if !strings.Contains(err.Error(), "no AI configured") {
		t.Errorf("runAIChat error = %v, want 'no AI configured'", err)
	}
}

func TestRunExternalCommand_RunsSuccessfully(t *testing.T) {
	if err := runExternalCommand("true"); err != nil {
		t.Errorf("runExternalCommand(true) failed: %v", err)
	}
}

func TestRunExternalCommand_MissingBinaryErrors(t *testing.T) {
	if err := runExternalCommand("devx-definitely-not-a-real-binary"); err == nil {
		t.Error("expected an error for a nonexistent binary")
	}
}

func TestRunAI_NoArgsLaunchesWizardOrConfiguredTool(t *testing.T) {
	// With no saved config, runAI falls into the first-time wizard.
	// Feed blank lines to accept every default, decline the local
	// backend setup ("n"), accept the default tool, then decline the
	// final "launch now?" prompt ("n") so nothing is actually executed.
	chdirTemp(t)
	withStdin(t, "\n\n\n\nn\n\nn\n")
	var err error
	out := captureStdout(t, func() {
		err = runAI(context.Background(), nil)
	})
	if err != nil {
		t.Fatalf("runAI() failed: %v", err)
	}
	if !strings.Contains(out, "Welcome to devx AI setup") {
		t.Errorf("expected the wizard welcome message, got:\n%s", out)
	}
}
