package main

import (
	"bufio"
	"strings"
	"testing"

	"github.com/dever-labs/devx/internal/localai"
)

func TestWizardLocal_AcceptAllDefaultsAndDeclineSetup(t *testing.T) {
	// backend (default), main model (default), autocomplete model (default),
	// "Start backend setup now?" -> decline, to avoid any real side effects.
	reader := bufio.NewReader(strings.NewReader("\n\n\nn\n"))

	var cfg *localai.SavedAIConfig
	var err error
	out := captureStdout(t, func() {
		cfg, err = wizardLocal(reader)
	})
	if err != nil {
		t.Fatalf("wizardLocal failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected a non-nil config")
	}
	if cfg.Provider != "local" {
		t.Errorf("cfg.Provider = %q, want local", cfg.Provider)
	}
	if cfg.Backend != localai.BackendMLX && cfg.Backend != localai.BackendOllama {
		t.Errorf("cfg.Backend = %q, want mlx or ollama", cfg.Backend)
	}
	if cfg.Model == "" {
		t.Error("expected a non-empty main model")
	}
	if !strings.Contains(out, "devx ai setup") {
		t.Errorf("expected a hint about running 'devx ai setup' later, got:\n%s", out)
	}
}

func TestWizardRemote_UnreachableEndpointStillReturnsConfig(t *testing.T) {
	// endpoint (default), model (default). The endpoint is intentionally
	// unreachable so this exercises the "not reachable" path deterministically.
	reader := bufio.NewReader(strings.NewReader("\n\n"))

	var cfg *localai.SavedAIConfig
	var err error
	out := captureStdout(t, func() {
		cfg, err = wizardRemote(reader)
	})
	if err != nil {
		t.Fatalf("wizardRemote failed: %v", err)
	}
	if cfg == nil || cfg.Provider != "remote" {
		t.Fatalf("expected a remote config, got %+v", cfg)
	}
	if cfg.Endpoint == "" || cfg.Model == "" {
		t.Errorf("expected non-empty endpoint/model, got %+v", cfg)
	}
	if !strings.Contains(out, "not reachable") {
		t.Errorf("expected 'not reachable' for an unreachable endpoint, got:\n%s", out)
	}
}

func TestWizardCloud_PicksAnthropicAndModel(t *testing.T) {
	// cloud provider (default -> anthropic), model (default).
	reader := bufio.NewReader(strings.NewReader("\n\n"))

	var cfg *localai.SavedAIConfig
	var err error
	_ = captureStdout(t, func() {
		cfg, err = wizardCloud(reader)
	})
	if err != nil {
		t.Fatalf("wizardCloud failed: %v", err)
	}
	if cfg == nil || cfg.Provider != "anthropic" {
		t.Fatalf("expected an anthropic config, got %+v", cfg)
	}
	if cfg.Model == "" {
		t.Error("expected a non-empty model")
	}
}

func TestWizardPickModel_DefaultSelection(t *testing.T) {
	models := []localai.ModelSuggestion{
		{Name: "a", Description: "first"},
		{Name: "b", Description: "second"},
	}
	reader := bufio.NewReader(strings.NewReader("\n"))
	var name string
	var err error
	_ = captureStdout(t, func() {
		name, err = wizardPickModel(reader, "Pick", models, "b")
	})
	if err != nil {
		t.Fatalf("wizardPickModel failed: %v", err)
	}
	if name != "b" {
		t.Errorf("wizardPickModel default = %q, want %q", name, "b")
	}
}

func TestWizardPickModel_CustomName(t *testing.T) {
	models := []localai.ModelSuggestion{{Name: "a", Description: "first"}}
	// Select the last option ("Enter a custom model name"), then type a name.
	reader := bufio.NewReader(strings.NewReader("2\nmy-custom-model\n"))
	var name string
	var err error
	_ = captureStdout(t, func() {
		name, err = wizardPickModel(reader, "Pick", models, "a")
	})
	if err != nil {
		t.Fatalf("wizardPickModel failed: %v", err)
	}
	if name != "my-custom-model" {
		t.Errorf("wizardPickModel custom = %q, want %q", name, "my-custom-model")
	}
}

func TestWizardTool_ReturnsToolIDForProvider(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\n"))
	var tool string
	var err error
	_ = captureStdout(t, func() {
		tool, err = wizardTool(reader, "local")
	})
	if err != nil {
		t.Fatalf("wizardTool failed: %v", err)
	}
	if tool == "" {
		t.Error("expected a non-empty tool ID")
	}
}
