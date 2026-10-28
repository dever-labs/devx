package localai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildContinueConfig_LocalOllamaDefaultModel(t *testing.T) {
	cfg := &SavedAIConfig{Provider: "local", Backend: BackendOllama}
	out, err := buildContinueConfig(cfg)
	if err != nil {
		t.Fatalf("buildContinueConfig: %v", err)
	}
	if len(out.Models) != 1 || out.Models[0].Provider != "ollama" {
		t.Errorf("expected a single ollama model, got %+v", out.Models)
	}
	if out.Models[0].Model != "deepseek-r1:70b" {
		t.Errorf("expected default main model, got %q", out.Models[0].Model)
	}
	if out.TabAutocompleteModel != nil {
		t.Errorf("expected no autocomplete model when AutocompleteModel is unset, got %+v", out.TabAutocompleteModel)
	}
}

func TestBuildContinueConfig_LocalOllamaWithAutocomplete(t *testing.T) {
	cfg := &SavedAIConfig{Provider: "local", Backend: BackendOllama, Model: "qwen2.5-coder:14b", AutocompleteModel: "qwen2.5-coder:1.5b"}
	out, err := buildContinueConfig(cfg)
	if err != nil {
		t.Fatalf("buildContinueConfig: %v", err)
	}
	if out.TabAutocompleteModel == nil || out.TabAutocompleteModel.Model != "qwen2.5-coder:1.5b" {
		t.Errorf("expected autocomplete model to be set, got %+v", out.TabAutocompleteModel)
	}
}

func TestBuildContinueConfig_LocalMLXUsesOpenAIProvider(t *testing.T) {
	cfg := &SavedAIConfig{Provider: "local", Backend: BackendMLX, Model: "mistral", AutocompleteModel: "small-model"}
	out, err := buildContinueConfig(cfg)
	if err != nil {
		t.Fatalf("buildContinueConfig: %v", err)
	}
	if len(out.Models) != 1 || out.Models[0].Provider != "openai" || out.Models[0].APIBase != "http://localhost:8080/v1" {
		t.Errorf("expected MLX model with default apiBase, got %+v", out.Models[0])
	}
	if out.TabAutocompleteModel == nil || out.TabAutocompleteModel.APIBase != "http://localhost:8080/v1" {
		t.Errorf("expected MLX autocomplete model with apiBase set, got %+v", out.TabAutocompleteModel)
	}
}

func TestBuildContinueConfig_LocalMLXCustomEndpoint(t *testing.T) {
	cfg := &SavedAIConfig{Provider: "local", Backend: BackendMLX, Model: "mistral", Endpoint: "http://custom:9000/v1"}
	out, err := buildContinueConfig(cfg)
	if err != nil {
		t.Fatalf("buildContinueConfig: %v", err)
	}
	if out.Models[0].APIBase != "http://custom:9000/v1" {
		t.Errorf("expected custom apiBase to be honored, got %q", out.Models[0].APIBase)
	}
}

func TestBuildContinueConfig_Remote(t *testing.T) {
	cfg := &SavedAIConfig{Provider: "remote", Endpoint: "http://ai.internal:11434"}
	out, err := buildContinueConfig(cfg)
	if err != nil {
		t.Fatalf("buildContinueConfig: %v", err)
	}
	if out.Models[0].Model != "qwen2.5-coder:14b" {
		t.Errorf("expected default remote model, got %q", out.Models[0].Model)
	}
	if out.Models[0].APIBase != "http://ai.internal:11434/v1" {
		t.Errorf("expected remote apiBase to append /v1, got %q", out.Models[0].APIBase)
	}
}

func TestBuildContinueConfig_Anthropic(t *testing.T) {
	cfg := &SavedAIConfig{Provider: "anthropic"}
	out, err := buildContinueConfig(cfg)
	if err != nil {
		t.Fatalf("buildContinueConfig: %v", err)
	}
	if out.Models[0].Provider != "anthropic" || out.Models[0].Model != "claude-3-5-sonnet-latest" {
		t.Errorf("expected default anthropic model, got %+v", out.Models[0])
	}
}

func TestBuildContinueConfig_OpenAI(t *testing.T) {
	cfg := &SavedAIConfig{Provider: "openai", Model: "o3"}
	out, err := buildContinueConfig(cfg)
	if err != nil {
		t.Fatalf("buildContinueConfig: %v", err)
	}
	if out.Models[0].Model != "o3" {
		t.Errorf("expected explicit model to be honored, got %q", out.Models[0].Model)
	}
}

func TestBuildContinueConfig_UnsupportedProviderErrors(t *testing.T) {
	cfg := &SavedAIConfig{Provider: "copilot"}
	if _, err := buildContinueConfig(cfg); err == nil {
		t.Fatal("expected an error for an unsupported provider")
	}
}

func TestWriteContinueConfig_WritesValidJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &SavedAIConfig{Provider: "openai", Model: "gpt-4o"}
	if err := WriteContinueConfig(cfg); err != nil {
		t.Fatalf("WriteContinueConfig: %v", err)
	}

	path := filepath.Join(home, ".continue", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written config: %v", err)
	}
	var parsed continueConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("written config is not valid JSON: %v", err)
	}
	if len(parsed.Models) != 1 || parsed.Models[0].Model != "gpt-4o" {
		t.Errorf("parsed config = %+v, want a single gpt-4o model", parsed)
	}
}

func TestWriteContinueConfig_UnsupportedProviderErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := WriteContinueConfig(&SavedAIConfig{Provider: "copilot"}); err == nil {
		t.Fatal("expected an error for an unsupported provider")
	}
}
