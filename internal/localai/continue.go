package localai

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteContinueConfig writes a minimal ~/.continue/config.json based on the
// saved AI config so that the Continue.dev VS Code extension connects to the
// correct backend automatically.
func WriteContinueConfig(cfg *SavedAIConfig) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".continue")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.json")

	content, err := buildContinueConfig(cfg)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	fmt.Printf("✓  wrote Continue.dev config: %s\n", path)
	return nil
}

type continueConfig struct {
	Models              []continueModel  `json:"models"`
	TabAutocompleteModel *continueModel  `json:"tabAutocompleteModel,omitempty"`
}

type continueModel struct {
	Title    string `json:"title"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIBase  string `json:"apiBase,omitempty"`
	APIKey   string `json:"apiKey,omitempty"`
}

func buildContinueConfig(cfg *SavedAIConfig) (*continueConfig, error) {
	out := &continueConfig{}

	switch cfg.Provider {
	case "local":
		backend := ResolveBackend(cfg.Backend)
		model := cfg.Model
		if model == "" {
			model = "deepseek-r1:70b"
		}

		if backend == BackendMLX {
			apiBase := "http://localhost:8080/v1"
			if cfg.Endpoint != "" {
				apiBase = cfg.Endpoint
			}
			out.Models = []continueModel{{
				Title:    "MLX — " + model,
				Provider: "openai",
				Model:    model,
				APIBase:  apiBase,
				APIKey:   "local",
			}}
			if cfg.AutocompleteModel != "" {
				ac := continueModel{
					Title:    "MLX autocomplete — " + cfg.AutocompleteModel,
					Provider: "openai",
					Model:    cfg.AutocompleteModel,
					APIBase:  apiBase,
					APIKey:   "local",
				}
				out.TabAutocompleteModel = &ac
			}
		} else {
			out.Models = []continueModel{{
				Title:    "Ollama — " + model,
				Provider: "ollama",
				Model:    model,
			}}
			if cfg.AutocompleteModel != "" {
				ac := continueModel{
					Title:    "Ollama autocomplete — " + cfg.AutocompleteModel,
					Provider: "ollama",
					Model:    cfg.AutocompleteModel,
				}
				out.TabAutocompleteModel = &ac
			}
		}

	case "remote":
		model := cfg.Model
		if model == "" {
			model = "qwen2.5-coder:14b"
		}
		apiBase := cfg.Endpoint
		out.Models = []continueModel{{
			Title:    "Remote — " + model,
			Provider: "openai",
			Model:    model,
			APIBase:  apiBase + "/v1",
			APIKey:   "remote",
		}}

	case "anthropic":
		model := cfg.Model
		if model == "" {
			model = "claude-3-5-sonnet-latest"
		}
		out.Models = []continueModel{{
			Title:    "Claude — " + model,
			Provider: "anthropic",
			Model:    model,
		}}

	case "openai":
		model := cfg.Model
		if model == "" {
			model = "gpt-4o"
		}
		out.Models = []continueModel{{
			Title:    "OpenAI — " + model,
			Provider: "openai",
			Model:    model,
		}}

	default:
		return nil, fmt.Errorf("unsupported provider for Continue.dev config: %s", cfg.Provider)
	}

	return out, nil
}
