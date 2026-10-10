package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func runAuth(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("auth", flag.ExitOnError)
	outputJSON := fs.Bool("json", false, "Emit auth status as JSON")
	_ = fs.Parse(args)

	checks := []authCheck{
		{"GitHub CLI", checkGitHubAuth},
		{"Anthropic", checkEnvKey("ANTHROPIC_API_KEY", "https://console.anthropic.com/settings/keys")},
		{"OpenAI", checkEnvKey("OPENAI_API_KEY", "https://platform.openai.com/api-keys")},
		{"Azure OpenAI", checkEnvKey("AZURE_OPENAI_KEY", "https://portal.azure.com")},
		{"Codex / OpenAI", checkEnvKey("OPENAI_API_KEY", "")},
	}

	// Deduplicate: only show OpenAI once.
	seen := map[string]bool{}
	type result struct {
		Name string `json:"name"`
		OK   bool   `json:"ok"`
		Hint string `json:"hint,omitempty"`
	}
	var results []result
	for _, c := range checks {
		if seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		status, hint := c.check()
		results = append(results, result{Name: c.Name, OK: status, Hint: hint})
	}

	if *outputJSON {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Println("Auth status")
	fmt.Println()
	for _, r := range results {
		if r.Hint != "" {
			fmt.Printf("  %s  %-20s  %s\n", statusIcon(r.OK), r.Name, r.Hint)
		} else {
			fmt.Printf("  %s  %s\n", statusIcon(r.OK), r.Name)
		}
	}

	fmt.Println()
	fmt.Println("To add a provider, run  devx ai  to set it up interactively.")
	return nil
}

type authCheck struct {
	Name  string
	check func() (bool, string)
}

func statusIcon(ok bool) string {
	if ok {
		return "✓"
	}
	return "✗"
}

// checkGitHubAuth runs `gh auth status` and returns true if authenticated.
func checkGitHubAuth() (bool, string) {
	if _, err := exec.LookPath("gh"); err != nil {
		return false, "gh CLI not installed — brew install gh"
	}
	out, err := exec.Command("gh", "auth", "status").CombinedOutput()
	if err != nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, l := range lines {
			if strings.Contains(l, "Logged in") || strings.Contains(l, "oauth_token") {
				return true, ""
			}
		}
		return false, "not authenticated — run: gh auth login"
	}
	// Extract account name if possible.
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "Logged in to") {
			account := ""
			parts := strings.Fields(line)
			for i, p := range parts {
				if p == "account" && i+1 < len(parts) {
					account = parts[i+1]
				}
			}
			if account != "" {
				return true, "account: " + account
			}
			return true, strings.TrimSpace(line)
		}
	}
	return true, ""
}

// checkEnvKey returns a checker that verifies an environment variable is set.
func checkEnvKey(envVar, keyURL string) func() (bool, string) {
	return func() (bool, string) {
		val := os.Getenv(envVar)
		if val == "" {
			hint := envVar + " not set"
			if keyURL != "" {
				hint += " — " + keyURL
			}
			return false, hint
		}
		return true, envVar + "=" + maskSecret(val)
	}
}
