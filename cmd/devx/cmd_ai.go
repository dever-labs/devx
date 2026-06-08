package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

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
	case "model":
		return runAIModel(rest)
	case "chat":
		return runAIChat(rest)
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
			"Remote  — shared team inference server (Ollama/vLLM endpoint)",
			"Cloud   — hosted API (Claude, OpenAI, GitHub Copilot)",
		}, 0)
	if err != nil {
		return err
	}
	fmt.Println()

	var cfg *localai.SavedAIConfig
	switch providerTypeIdx {
	case 0:
		cfg, err = wizardLocal(reader)
	case 1:
		cfg, err = wizardRemote(reader)
	default:
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

	// Write Continue.dev config so the VS Code extension connects automatically.
	if err := localai.WriteContinueConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write Continue.dev config: %v\n", err)
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

// ── Remote wizard path ────────────────────────────────────────────────────────────────────

func wizardRemote(reader *bufio.Reader) (*localai.SavedAIConfig, error) {
	fmt.Println("Enter the base URL of your team's inference server.")
	fmt.Println("Examples:")
	fmt.Println("  http://ai.internal:11434   (Ollama)")
	fmt.Println("  http://ai.internal:8080    (MLX-LM)")
	fmt.Println("  http://vllm.internal:8000  (vLLM)")
	fmt.Println()

	endpoint, err := promptString(reader, "Endpoint URL", "http://ai.internal:11434")
	if err != nil {
		return nil, err
	}
	fmt.Println()

	// Probe connectivity.
	fmt.Printf("Checking %s ... ", endpoint)
	status, _ := localai.DetectWithEndpoint(localai.BackendRemote, endpoint)
	if status != nil {
		fmt.Printf("✓  reachable (%s)\n", status.Provider)
	} else {
		fmt.Println("✗  not reachable — you can continue but AI will fail until it's up")
	}
	fmt.Println()

	model, err := promptString(reader, "Default model name", "qwen2.5-coder:14b")
	if err != nil {
		return nil, err
	}
	fmt.Println()

	return &localai.SavedAIConfig{
		Provider: "remote",
		Backend:  localai.BackendRemote,
		Endpoint: endpoint,
		Model:    model,
	}, nil
}

// ── Cloud wizard path ──────────────────────────────────────────────────────────────────────

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

	if err := localai.Setup(opts); err != nil {
		return err
	}

	// Refresh Continue.dev config with current settings.
	if saved, err := localai.LoadSavedAIConfig(); err == nil && saved != nil {
		if writeErr := localai.WriteContinueConfig(saved); writeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not update Continue.dev config: %v\n", writeErr)
		}
	}
	return nil
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

// ── Chat ─────────────────────────────────────────────────────────────────────────────────────

// runAIChat sends a one-off question to the configured AI backend and streams the response.
func runAIChat(args []string) error {
	question := strings.Join(args, " ")
	if question == "" {
		reader := bufio.NewReader(os.Stdin)
		var err error
		question, err = promptString(reader, "Ask anything", "")
		if err != nil || question == "" {
			return nil
		}
	}

	cfg, err := localai.LoadSavedAIConfig()
	if err != nil {
		return fmt.Errorf("reading AI config: %w", err)
	}
	if cfg == nil {
		return fmt.Errorf("no AI configured — run  devx ai  to set things up")
	}

	switch cfg.Provider {
	case "local", "remote":
		return chatViaOpenAIAPI(cfg, question)
	case "anthropic":
		// Delegate to claude CLI non-interactively.
		return runExternalCommand("claude", "-p", question)
	case "openai":
		return runExternalCommand("codex", "ask", question)
	case "copilot":
		return runExternalCommand("gh", "copilot", "explain", question)
	default:
		return fmt.Errorf("chat not supported for provider %q", cfg.Provider)
	}
}

func chatViaOpenAIAPI(cfg *localai.SavedAIConfig, question string) error {
	status, err := localai.DetectWithEndpoint(cfg.Backend, cfg.Endpoint)
	if err != nil || status == nil {
		return fmt.Errorf("AI backend not running — start with: devx ai setup")
	}

	model := cfg.Model
	if model == "" {
		model = status.Model
	}

	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": question},
		},
		"stream": false,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", status.APIURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer local")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	if result.Error != nil {
		return fmt.Errorf("API error: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return fmt.Errorf("empty response from model")
	}

	fmt.Println(result.Choices[0].Message.Content)
	return nil
}

// ── Model management ────────────────────────────────────────────────────────────────────────────

func runAIModel(args []string) error {
	if len(args) == 0 {
		fmt.Println("Usage:")
		fmt.Println("  devx ai model list")
		fmt.Println("  devx ai model pull <name>")
		fmt.Println("  devx ai model remove <name>")
		return nil
	}
	sub := args[0]
	rest := args[1:]

	cfg, _ := localai.LoadSavedAIConfig()
	backend := localai.BackendOllama
	if cfg != nil {
		backend = localai.ResolveBackend(cfg.Backend)
	}

	switch sub {
	case "list", "ls":
		return runAIModelList(backend, cfg)
	case "pull":
		if len(rest) == 0 {
			return fmt.Errorf("usage: devx ai model pull <name>")
		}
		return runAIModelPull(backend, rest[0])
	case "remove", "rm", "delete":
		if len(rest) == 0 {
			return fmt.Errorf("usage: devx ai model remove <name>")
		}
		return runAIModelRemove(backend, rest[0])
	default:
		return fmt.Errorf("unknown model subcommand: %s", sub)
	}
}

func runAIModelList(backend string, cfg *localai.SavedAIConfig) error {
	endpoint := ""
	if cfg != nil {
		endpoint = cfg.Endpoint
	}
	status, err := localai.DetectWithEndpoint(backend, endpoint)
	if err != nil {
		return err
	}
	if status == nil {
		return fmt.Errorf("no AI backend running — start it with: devx ai setup")
	}

	switch status.Backend {
	case localai.BackendOllama, localai.BackendRemote:
		fmt.Printf("Models on %s (%s):\n\n", status.URL, status.Backend)
		return runExternalCommand("ollama", "list")
	case localai.BackendMLX:
		fmt.Printf("Models loaded on MLX (%s):\n\n", status.URL)
		// MLX serves one model at a time — show it via the API.
		if status.Model != "" {
			fmt.Printf("  %s  (currently loaded)\n", status.Model)
		} else {
			fmt.Println("  (no model info available from API)")
		}
		fmt.Println()
		fmt.Println("Manage MLX models with:")
		fmt.Println("  python -m mlx_lm.convert --hf-path <hf-model-id>")
	}
	return nil
}

func runAIModelPull(backend, name string) error {
	switch backend {
	case localai.BackendOllama:
		fmt.Printf("Pulling %s via Ollama...\n", name)
		return runExternalCommand("ollama", "pull", name)
	case localai.BackendMLX:
		fmt.Printf("Pulling %s for MLX...\n", name)
		fmt.Println("MLX does not have a built-in pull command.")
		fmt.Println("Download and convert with:")
		fmt.Printf("  python -m mlx_lm.convert --hf-path %s --mlx-path ~/.cache/mlx/%s\n", name, name)
		return nil
	default:
		fmt.Printf("For remote backends, use your server's management tools to pull %s.\n", name)
		return nil
	}
}

func runAIModelRemove(backend, name string) error {
	switch backend {
	case localai.BackendOllama:
		fmt.Printf("Removing %s from Ollama...\n", name)
		return runExternalCommand("ollama", "rm", name)
	case localai.BackendMLX:
		fmt.Printf("To remove an MLX model, delete its directory:\n")
		fmt.Printf("  rm -rf ~/.cache/mlx/%s\n", name)
		return nil
	default:
		fmt.Printf("For remote backends, manage models on the server directly.\n")
		return nil
	}
}

func runExternalCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
	fmt.Println("  devx ai                      first-time wizard, or launch configured tool")
	fmt.Println("  devx ai chat [question]      one-off question to the configured AI")
	fmt.Println("  devx ai status               show current AI configuration and backend state")
	fmt.Println("  devx ai reset                clear saved config and re-run wizard next time")
	fmt.Println("  devx ai setup                (re)start the local inference backend")
	fmt.Println("    --backend mlx|ollama        force a specific backend")
	fmt.Println("    --model <name>              override main model")
	fmt.Println("    --autocomplete-model <name>")
	fmt.Println("  devx ai model list            list available models")
	fmt.Println("  devx ai model pull <name>     pull a model (Ollama)")
	fmt.Println("  devx ai model remove <name>   remove a model (Ollama)")
}
