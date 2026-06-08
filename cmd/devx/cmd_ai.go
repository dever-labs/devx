package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dever-labs/devx/internal/localai"
)

// ── Entry point ────────────────────────────────────────────────

func runAI(_ context.Context, args []string) error {
	if len(args) == 0 {
		return runAIDefault()
	}

	sub := args[0]
	rest := args[1:]

	switch sub {
	case "setup":
		return runAISetup(rest)
	case "status":
		return runAIStatus(rest)
	case "reset":
		return runAIReset()
	case "help", "-h", "--help":
		printAIUsage()
		return nil
	default:
		fmt.Fprintf(os.Stderr, "Unknown ai subcommand: %s\n", sub)
		printAIUsage()
		return fmt.Errorf("unknown ai subcommand: %s", sub)
	}
}

// ── Default (devx ai with no args) ────────────────────────────────────────────────

// runAIDefault either launches the configured tool or runs the first-time wizard.
func runAIDefault() error {
	cfg, err := localai.LoadSavedAIConfig()
	if err != nil {
		return fmt.Errorf("reading AI config: %w", err)
	}
	if cfg != nil {
		return localai.LaunchTool(cfg)
	}
	// No config — first-time wizard.
	reader := bufio.NewReader(os.Stdin)
	return runAIWizard(reader)
}

// ── Wizard ────────────────────────────────────────────────────────────────────────────

func runAIWizard(reader *bufio.Reader) error {
	fmt.Println("Welcome to devx AI setup!")
	fmt.Println()
	fmt.Println("This wizard gets you up and running with AI in your dev environment.")
	fmt.Println("Run  devx ai reset  at any time to change your setup.")
	fmt.Println()

	providerTypeIdx, err := promptChoice(reader,
		"Where do you want to run AI?",
		[]string{
			"Local   — models run on this machine, private, no API costs",
			"Cloud   — hosted API (Claude, OpenAI, GitHub Copilot)",
		}, 0)
	if err != nil {
		return err
	}
	fmt.Println()

	var cfg *localai.SavedAIConfig
	if providerTypeIdx == 0 {
		cfg, err = wizardLocal(reader)
	} else {
		cfg, err = wizardCloud(reader)
	}
	if err != nil || cfg == nil {
		return err
	}

	// Tool selection
	cfg.Tool, err = wizardTool(reader, cfg.Provider)
	if err != nil {
		return err
	}
	fmt.Println()

	// Save
	if err := localai.SaveAIConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save config: %v\n", err)
	} else {
		fmt.Printf("Config saved to %s\n", localai.AIConfigPath)
	}
	fmt.Println()

	// Print devx.yaml snippet
	printYAMLSnippet(cfg)
	fmt.Println()

	// Offer to launch now
	launch, err := promptYN(reader, "Launch "+cfg.Tool+" now?", true)
	if err != nil || !launch {
		fmt.Println("Run  devx ai  any time to start it.")
		return err
	}
	fmt.Println()
	return localai.LaunchTool(cfg)
}

// ── Local wizard path ────────────────────────────────────────────────────────────────────

func wizardLocal(reader *bufio.Reader) (*localai.SavedAIConfig, error) {
	defaultBackend := localai.ResolveBackend(localai.BackendAuto)
	backendOpts := []string{
		"MLX    — Apple Silicon optimized, 20–50% faster than Ollama",
		"Ollama — works on any platform (Linux, Intel Mac, Windows)",
	}
	defaultBackendIdx := 0
	if defaultBackend == localai.BackendOllama {
		defaultBackendIdx = 1
	}
	backendIdx, err := promptChoice(reader, "Which local backend?", backendOpts, defaultBackendIdx)
	if err != nil {
		return nil, err
	}
	backend := localai.BackendMLX
	if backendIdx == 1 {
		backend = localai.BackendOllama
	}
	fmt.Println()

	// Main model
	mainModel, err := wizardPickModel(reader,
		"Main model  (chat, reasoning, coding tasks)",
		localai.MainModels(), "deepseek-r1:70b")
	if err != nil {
		return nil, err
	}
	fmt.Println()

	// Autocomplete model
	acOpts := append(localai.AutocompleteModels(), localai.ModelSuggestion{
		Name: "none", Description: "skip autocomplete",
	})
	acModel, err := wizardPickModel(reader,
		"Autocomplete model  (inline tab completion — pick a fast one)",
		acOpts, "qwen2.5-coder:1.5b")
	if err != nil {
		return nil, err
	}
	if acModel == "none" {
		acModel = ""
	}
	fmt.Println()

	// Summary + setup
	fmt.Println("──────────────────────────────────────────")
	fmt.Printf("  Backend            : %s\n", backend)
	fmt.Printf("  Main model         : %s\n", mainModel)
	if acModel != "" {
		fmt.Printf("  Autocomplete model : %s\n", acModel)
	}
	fmt.Println()

	startNow, err := promptYN(reader, "Start backend setup now?", true)
	if err != nil {
		return nil, err
	}
	if startNow {
		fmt.Println()
		if err := localai.Setup(localai.SetupOptions{
			Backend:           backend,
			Model:             mainModel,
			AutocompleteModel: acModel,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "warning: setup returned error: %v\n", err)
			fmt.Println("You can retry later with: devx ai setup")
		}
	} else {
		fmt.Println("You can start the backend later with: devx ai setup")
	}
	fmt.Println()

	return &localai.SavedAIConfig{
		Provider:          "local",
		Backend:           backend,
		Model:             mainModel,
		AutocompleteModel: acModel,
	}, nil
}

// ── Cloud wizard path ────────────────────────────────────────────────────────────────────

func wizardCloud(reader *bufio.Reader) (*localai.SavedAIConfig, error) {
	providers := localai.DetectCloudProviders()

	labels := make([]string, len(providers))
	for i, p := range providers {
		status := ""
		if p.Configured {
			status = "  ✓ configured"
		}
		labels[i] = p.Title + status
	}

	idx, err := promptChoice(reader, "Which cloud provider?", labels, 0)
	if err != nil {
		return nil, err
	}
	chosen := providers[idx]
	fmt.Println()

	if chosen.EnvVar != "" {
		if os.Getenv(chosen.EnvVar) != "" {
			fmt.Printf("  %s ✓\n", chosen.EnvVar)
		} else {
			fmt.Printf("  %s ✗  not set\n", chosen.EnvVar)
			if chosen.KeyURL != "" {
				fmt.Printf("  Get a key: %s\n", chosen.KeyURL)
			}
			fmt.Println()
			fmt.Println("  Add to your shell profile (~/.zshrc or ~/.bashrc):")
			fmt.Printf("    export %s=your-key-here\n", chosen.EnvVar)
		}
	} else if chosen.Name == "copilot" {
		if chosen.Configured {
			fmt.Println("  GitHub CLI: ✓ authenticated")
		} else {
			fmt.Println("  GitHub CLI: ✗ not authenticated — run: gh auth login")
		}
	}
	fmt.Println()

	model := ""
	if cloudModels := localai.CloudModels(chosen.Name); len(cloudModels) > 0 {
		model, err = wizardPickModel(reader, "Which model?", cloudModels, cloudModels[0].Name)
		if err != nil {
			return nil, err
		}
		fmt.Println()
	}

	return &localai.SavedAIConfig{
		Provider: chosen.Name,
		Model:    model,
	}, nil
}

// ── Shared wizard helpers ────────────────────────────────────────────────────────────────────────────

func wizardPickModel(reader *bufio.Reader, label string, models []localai.ModelSuggestion, defaultModel string) (string, error) {
	labels := make([]string, len(models)+1)
	defaultIdx := 0
	for i, m := range models {
		col1 := fmt.Sprintf("%-28s", m.Name)
		desc := m.Description
		if m.VRAM != "" {
			desc += fmt.Sprintf("  (%s)", m.VRAM)
		}
		labels[i] = col1 + "  " + desc
		if m.Name == defaultModel {
			defaultIdx = i
		}
	}
	labels[len(models)] = "Enter a custom model name"

	idx, err := promptChoice(reader, label, labels, defaultIdx)
	if err != nil {
		return "", err
	}
	if idx < len(models) {
		return models[idx].Name, nil
	}
	return promptString(reader, "Model name", defaultModel)
}

func wizardTool(reader *bufio.Reader, provider string) (string, error) {
	tools := localai.ToolsForProvider(provider)
	labels := make([]string, len(tools))
	for i, t := range tools {
		labels[i] = fmt.Sprintf("%-14s %s", t.Title, t.Desc)
	}
	idx, err := promptChoice(reader, "Which tool do you want to launch with  devx ai?", labels, 0)
	if err != nil {
		return "", err
	}
	return tools[idx].ID, nil
}

func printYAMLSnippet(cfg *localai.SavedAIConfig) {
	fmt.Println("To share this setup with your team, add to devx.yaml:")
	fmt.Println()
	fmt.Println("  ai:")
	if cfg.Provider != "" {
		fmt.Printf("    provider: %s\n", cfg.Provider)
	}
	if cfg.Model != "" {
		fmt.Printf("    model: %s\n", cfg.Model)
	}
	if cfg.Provider == "local" {
		fmt.Println("    local:")
		fmt.Printf("      backend: %s\n", cfg.Backend)
		if cfg.AutocompleteModel != "" {
			fmt.Printf("      autocompleteModel: %s\n", cfg.AutocompleteModel)
		}
	}
}

// ── Subcommands ────────────────────────────────────────────────────────────────────────────

func runAISetup(args []string) error {
	fs := flag.NewFlagSet("ai setup", flag.ExitOnError)
	backend := fs.String("backend", "", "Backend: mlx | ollama (default: auto)")
	model := fs.String("model", "", "Override main model")
	autocomplete := fs.String("autocomplete-model", "", "Override autocomplete model")
	_ = fs.Parse(args)

	opts := localai.SetupOptions{
		Backend:           *backend,
		Model:             *model,
		AutocompleteModel: *autocomplete,
	}

	// Fall back to saved config, then devx.yaml
	if saved, err := localai.LoadSavedAIConfig(); err == nil && saved != nil {
		if opts.Backend == "" {
			opts.Backend = saved.Backend
		}
		if opts.Model == "" {
			opts.Model = saved.Model
		}
		if opts.AutocompleteModel == "" {
			opts.AutocompleteModel = saved.AutocompleteModel
		}
	} else if manifest, err := loadManifestOnly(); err == nil && manifest.AI != nil && manifest.AI.Local != nil {
		local := manifest.AI.Local
		if opts.Backend == "" {
			opts.Backend = local.Backend
		}
		if opts.Model == "" {
			opts.Model = local.Model
		}
		if opts.Model == "" {
			opts.Model = manifest.AI.Model
		}
		if opts.AutocompleteModel == "" {
			opts.AutocompleteModel = local.AutocompleteModel
		}
	}

	return localai.Setup(opts)
}

func runAIStatus(args []string) error {
	fs := flag.NewFlagSet("ai status", flag.ExitOnError)
	_ = fs.Parse(args)

	cfg, err := localai.LoadSavedAIConfig()
	if err != nil {
		return fmt.Errorf("reading AI config: %w", err)
	}

	if cfg != nil {
		fmt.Println("Saved config  (.devx/ai.yaml)")
		fmt.Printf("  Provider  : %s\n", cfg.Provider)
		if cfg.Backend != "" {
			fmt.Printf("  Backend   : %s\n", cfg.Backend)
		}
		if cfg.Model != "" {
			fmt.Printf("  Model     : %s\n", cfg.Model)
		}
		if cfg.Tool != "" {
			fmt.Printf("  Tool      : %s\n", cfg.Tool)
		}
		fmt.Println()
	}

	if cfg == nil || cfg.Provider == "local" {
		fmt.Print("Local backend... ")
		backend, _ := localai.Detect(localai.BackendAuto)
		if backend != nil {
			fmt.Printf("✓  %s  %s\n", backend.Backend, backend.APIURL)
			if backend.Model != "" {
				fmt.Printf("              model: %s\n", backend.Model)
			}
		} else {
			fmt.Println("✗  not running  (devx ai setup to start)")
		}
		fmt.Println()
	}

	if cfg == nil {
		fmt.Println("No AI configured — run  devx ai  to get started.")
	}
	return nil
}

func runAIReset() error {
	cfg, _ := localai.LoadSavedAIConfig()
	if cfg == nil {
		fmt.Println("No AI config found — nothing to reset.")
		fmt.Println("Run  devx ai  to set things up.")
		return nil
	}

	reader := bufio.NewReader(os.Stdin)
	confirm, err := promptYN(reader,
		fmt.Sprintf("Reset AI config (provider: %s, tool: %s)?", cfg.Provider, cfg.Tool),
		false)
	if err != nil || !confirm {
		fmt.Println("Aborted.")
		return err
	}

	if err := localai.DeleteAIConfig(); err != nil {
		return fmt.Errorf("deleting config: %w", err)
	}
	fmt.Println("AI config cleared. Run  devx ai  to set things up again.")
	return nil
}

// ── Prompt helpers ────────────────────────────────────────────────────────────────────────────

func promptChoice(reader *bufio.Reader, label string, options []string, defaultIdx int) (int, error) {
	fmt.Println(label)
	for i, o := range options {
		marker := " "
		if i == defaultIdx {
			marker = "›"
		}
		fmt.Printf("  %s %d) %s\n", marker, i+1, o)
	}
	fmt.Printf("Choice [%d]: ", defaultIdx+1)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultIdx, nil
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(options) {
		fmt.Printf("  Invalid — using default (%d)\n", defaultIdx+1)
		return defaultIdx, nil
	}
	return n - 1, nil
}

func promptString(reader *bufio.Reader, label, defaultVal string) (string, error) {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal, nil
	}
	return line, nil
}

func promptYN(reader *bufio.Reader, label string, defaultYes bool) (bool, error) {
	hint := "[Y/n]"
	if !defaultYes {
		hint = "[y/N]"
	}
	fmt.Printf("%s %s ", label, hint)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultYes, nil
	}
	return line == "y" || line == "yes", nil
}

// ── Usage ────────────────────────────────────────────────────────────────────────────

func printAIUsage() {
	fmt.Println("devx ai - set up and manage AI for your dev environment")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  devx ai                    first-time wizard, or launch configured tool")
	fmt.Println("  devx ai status             show current AI configuration and backend state")
	fmt.Println("  devx ai reset              clear saved config and re-run wizard next time")
	fmt.Println("  devx ai setup              (re)start the local inference backend")
	fmt.Println("    --backend mlx|ollama     force a specific backend")
	fmt.Println("    --model <name>           override main model")
	fmt.Println("    --autocomplete-model <name>")
}
