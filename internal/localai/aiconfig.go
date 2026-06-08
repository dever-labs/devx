package localai

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

const AIConfigPath = ".devx/ai.yaml"

// SavedAIConfig is persisted to .devx/ai.yaml after first-time setup.
// It drives both  devx ai  (launch) and  devx ai status  (info).
type SavedAIConfig struct {
	// Provider: local | anthropic | openai | copilot
	Provider string `yaml:"provider"`
	// Tool to launch when running  devx ai: aider | claude-code | codex | gh-copilot
	Tool string `yaml:"tool"`
	// Model is the main chat/reasoning model
	Model string `yaml:"model,omitempty"`
	// Backend: mlx | ollama — only meaningful when Provider == "local"
	Backend string `yaml:"backend,omitempty"`
	// AutocompleteModel for tab completion in Continue.dev
	AutocompleteModel string `yaml:"autocompleteModel,omitempty"`
}

// LoadSavedAIConfig reads .devx/ai.yaml. Returns nil (no error) when absent.
func LoadSavedAIConfig() (*SavedAIConfig, error) {
	data, err := os.ReadFile(AIConfigPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg SavedAIConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("reading %s: %w", AIConfigPath, err)
	}
	return &cfg, nil
}

// SaveAIConfig writes cfg to .devx/ai.yaml, creating .devx/ if needed.
func SaveAIConfig(cfg *SavedAIConfig) error {
	if err := os.MkdirAll(".devx", 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	header := []byte("# Written by 'devx ai' — run 'devx ai reset' to reconfigure\n")
	return os.WriteFile(AIConfigPath, append(header, data...), 0644)
}

// DeleteAIConfig removes .devx/ai.yaml. No-op if absent.
func DeleteAIConfig() error {
	err := os.Remove(AIConfigPath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ToolOption describes a coding tool available for a given provider.
type ToolOption struct {
	ID    string // aider | claude-code | codex | gh-copilot
	Title string
	Desc  string
}

// ToolsForProvider returns the recommended tools for a given provider.
func ToolsForProvider(provider string) []ToolOption {
	switch provider {
	case "local":
		return []ToolOption{
			{"aider", "Aider", "autonomous agent — edits files, runs tests"},
			{"codex", "Codex CLI", "OpenAI-compatible CLI agent"},
		}
	case "anthropic":
		return []ToolOption{
			{"claude-code", "Claude Code", "Anthropic's official CLI agent"},
			{"aider", "Aider", "with Claude backend"},
		}
	case "openai":
		return []ToolOption{
			{"codex", "Codex CLI", "OpenAI's official CLI agent"},
			{"aider", "Aider", "with OpenAI backend"},
		}
	case "copilot":
		return []ToolOption{
			{"gh-copilot", "GitHub Copilot CLI", "gh copilot suggest / explain"},
		}
	default:
		return []ToolOption{
			{"aider", "Aider", "autonomous terminal agent"},
		}
	}
}

// LaunchTool starts the configured AI coding tool, blocking until it exits.
// If the local backend is configured but not running, it returns an error.
func LaunchTool(cfg *SavedAIConfig) error {
	// For local backends, verify the backend is up before launching.
	if cfg.Provider == "local" {
		backend, err := Detect(cfg.Backend)
		if err != nil {
			return fmt.Errorf("backend detection failed: %w", err)
		}
		if backend == nil {
			return fmt.Errorf(
				"local AI backend (%s) is not running — start it first with: devx ai setup",
				cfg.Backend,
			)
		}
	}

	cmd, err := buildToolCmd(cfg)
	if err != nil {
		return err
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func buildToolCmd(cfg *SavedAIConfig) (*exec.Cmd, error) {
	env := os.Environ()

	switch cfg.Tool {
	case "aider":
		args := aiderArgs(cfg)
		if cfg.Provider == "local" {
			if cfg.Backend == BackendMLX {
				env = setenv(env, "OPENAI_API_BASE", "http://localhost:8080/v1")
				env = setenv(env, "OPENAI_API_KEY", "local")
			} else {
				env = setenv(env, "OLLAMA_API_BASE", "http://localhost:11434")
			}
		}
		cmd := exec.Command("aider", args...)
		cmd.Env = env
		return cmd, nil

	case "claude-code":
		return exec.Command("claude"), nil

	case "codex":
		if cfg.Provider == "local" {
			env = setenv(env, "OPENAI_API_BASE", "http://localhost:8080/v1")
			env = setenv(env, "OPENAI_API_KEY", "local")
		}
		cmd := exec.Command("codex")
		cmd.Env = env
		return cmd, nil

	case "gh-copilot":
		return exec.Command("gh", "copilot", "suggest"), nil

	default:
		return nil, fmt.Errorf("unknown tool %q — run 'devx ai reset' to reconfigure", cfg.Tool)
	}
}

// aiderArgs builds  --model <tag>  for the configured provider/backend.
func aiderArgs(cfg *SavedAIConfig) []string {
	if cfg.Model == "" {
		return nil
	}
	switch cfg.Provider {
	case "local":
		if cfg.Backend == BackendMLX {
			return []string{"--model", "openai/" + cfg.Model}
		}
		return []string{"--model", "ollama/" + cfg.Model}
	case "anthropic":
		return []string{"--model", "anthropic/" + cfg.Model}
	case "openai":
		return []string{"--model", cfg.Model}
	default:
		return nil
	}
}

func setenv(env []string, key, val string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + val
			return env
		}
	}
	return append(env, prefix+val)
}
