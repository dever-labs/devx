package main

import (
	"os"
	"strings"
	"testing"
)

func writeTestAIConfig(dir, content string) error {
	if err := os.MkdirAll(dir+"/.devx", 0755); err != nil {
		return err
	}
	return os.WriteFile(dir+"/.devx/ai.yaml", []byte(content), 0644)
}

func TestRunAIStatus_NoConfigReportsNotConfigured(t *testing.T) {
	chdirTemp(t)
	var err error
	out := captureStdout(t, func() {
		err = runAIStatus(nil)
	})
	if err != nil {
		t.Fatalf("runAIStatus failed: %v", err)
	}
	if !strings.Contains(out, "No AI configured") {
		t.Errorf("expected 'No AI configured' message, got:\n%s", out)
	}
}

func TestRunAIReset_NoConfigPrintsNothingToReset(t *testing.T) {
	chdirTemp(t)
	var err error
	out := captureStdout(t, func() {
		err = runAIReset()
	})
	if err != nil {
		t.Fatalf("runAIReset failed: %v", err)
	}
	if !strings.Contains(out, "nothing to reset") {
		t.Errorf("expected 'nothing to reset' message, got:\n%s", out)
	}
}

func TestRunAIReset_DeclineAbortsWithoutDeleting(t *testing.T) {
	dir := chdirTemp(t)
	if err := writeTestAIConfig(dir, "provider: local\ntool: aider\n"); err != nil {
		t.Fatal(err)
	}
	withStdin(t, "n\n")

	var err error
	out := captureStdout(t, func() {
		err = runAIReset()
	})
	if err != nil {
		t.Fatalf("runAIReset failed: %v", err)
	}
	if !strings.Contains(out, "Aborted") {
		t.Errorf("expected 'Aborted' message, got:\n%s", out)
	}
	if !fileExists(dir + "/.devx/ai.yaml") {
		t.Error("expected config file to still exist after declining the reset")
	}
}

func TestRunAIReset_ConfirmDeletesConfig(t *testing.T) {
	dir := chdirTemp(t)
	if err := writeTestAIConfig(dir, "provider: local\ntool: aider\n"); err != nil {
		t.Fatal(err)
	}
	withStdin(t, "y\n")

	var err error
	_ = captureStdout(t, func() {
		err = runAIReset()
	})
	if err != nil {
		t.Fatalf("runAIReset failed: %v", err)
	}
	if fileExists(dir + "/.devx/ai.yaml") {
		t.Error("expected config file to be removed after confirming the reset")
	}
}
