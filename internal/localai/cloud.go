package localai

import (
	"os"
	"os/exec"
)

// CloudProvider describes a supported cloud AI provider and its current state.
type CloudProvider struct {
	Name       string   // anthropic | openai | copilot
	Title      string   // human-readable name
	EnvVar     string   // env var that holds the API key (empty for copilot)
	Configured bool     // whether credentials are present
	Tools      []string // recommended coding tools for this provider
	ModelEx    string   // example model name
	KeyURL     string   // where to get a key
}

// DetectCloudProviders checks which cloud AI providers are currently configured.
func DetectCloudProviders() []CloudProvider {
	providers := []CloudProvider{
		{
			Name:    "anthropic",
			Title:   "Anthropic Claude",
			EnvVar:  "ANTHROPIC_API_KEY",
			Tools:   []string{"claude-code", "aider"},
			ModelEx: "claude-sonnet-4-5",
			KeyURL:  "https://console.anthropic.com/settings/keys",
		},
		{
			Name:    "openai",
			Title:   "OpenAI",
			EnvVar:  "OPENAI_API_KEY",
			Tools:   []string{"codex", "aider"},
			ModelEx: "gpt-4o",
			KeyURL:  "https://platform.openai.com/api-keys",
		},
		{
			Name:  "copilot",
			Title: "GitHub Copilot",
			Tools: []string{"gh-copilot"},
		},
	}

	for i, p := range providers {
		if p.EnvVar != "" {
			providers[i].Configured = os.Getenv(p.EnvVar) != ""
		} else if p.Name == "copilot" {
			providers[i].Configured = isCopilotAuthenticated()
		}
	}

	return providers
}

func isCopilotAuthenticated() bool {
	if _, err := exec.LookPath("gh"); err != nil {
		return false
	}
	return exec.Command("gh", "auth", "status").Run() == nil
}
