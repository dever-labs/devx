package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ── Entry point ───────────────────────────────────────────────────────────────

func runDev(_ context.Context, args []string) error {
	if len(args) == 0 {
		return runDevStatus()
	}

	sub := args[0]
	rest := args[1:]

	switch sub {
	case "build":
		return runDevBuild(rest, false)
	case "rebuild":
		return runDevBuild(rest, true)
	case "up":
		return runDevUp(rest)
	case "open":
		return runDevOpen()
	case "exec":
		return runDevExec(rest)
	case "logs":
		return runDevLogs(rest)
	case "ps":
		return runDevPS()
	case "shell":
		return runDevShell()
	case "help", "-h", "--help":
		printDevUsage()
		return nil
	default:
		fmt.Fprintf(os.Stderr, "Unknown dev subcommand: %s\n", sub)
		printDevUsage()
		return fmt.Errorf("unknown dev subcommand: %s", sub)
	}
}

// ── Status ────────────────────────────────────────────────────────────────────

func runDevStatus() error {
	cfg, path, err := loadDevcontainerJSON()
	if err != nil {
		return err
	}

	cwd, _ := os.Getwd()
	fmt.Printf("Devcontainer  %s\n", cwd)
	fmt.Printf("Config        %s\n", path)
	fmt.Println()

	if cfg.Name != "" {
		fmt.Printf("  Name     : %s\n", cfg.Name)
	}
	if cfg.Image != "" {
		fmt.Printf("  Image    : %s\n", cfg.Image)
	}
	if cfg.Build != nil && cfg.Build.Dockerfile != "" {
		ctx := cfg.Build.Context
		if ctx == "" {
			ctx = "."
		}
		fmt.Printf("  Build    : %s  (context: %s)\n", cfg.Build.Dockerfile, ctx)
	}

	if len(cfg.Features) > 0 {
		fmt.Println()
		fmt.Println("  Features:")
		names := make([]string, 0, len(cfg.Features))
		for k := range cfg.Features {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, f := range names {
			fmt.Printf("    • %s\n", shortFeatureName(f))
		}
	}

	if len(cfg.ForwardPorts) > 0 {
		fmt.Println()
		ports := make([]string, len(cfg.ForwardPorts))
		for i, p := range cfg.ForwardPorts {
			ports[i] = fmt.Sprintf("%v", p)
		}
		fmt.Printf("  Ports    : %s\n", strings.Join(ports, ", "))
	}

	if len(cfg.Mounts) > 0 {
		fmt.Println()
		fmt.Println("  Mounts:")
		for _, m := range cfg.Mounts {
			fmt.Printf("    • %s\n", mountSummary(m))
		}
	}

	if cmds := lifecycleCommands(cfg); len(cmds) > 0 {
		fmt.Println()
		fmt.Println("  Lifecycle:")
		for _, c := range cmds {
			fmt.Printf("    • %s\n", c)
		}
	}

	if exts := vscodeExtensions(cfg); len(exts) > 0 {
		fmt.Println()
		fmt.Println("  VS Code extensions:")
		for _, e := range exts {
			fmt.Printf("    • %s\n", e)
		}
	}

	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  devx dev build    — build the image")
	fmt.Println("  devx dev rebuild  — rebuild without cache")
	fmt.Println("  devx dev up       — start the container")
	fmt.Println("  devx dev open     — open in VS Code / Cursor")
	return nil
}

// ── Build ─────────────────────────────────────────────────────────────────────

func runDevBuild(args []string, noCache bool) error {
	fs := flag.NewFlagSet("dev build", flag.ExitOnError)
	noCacheFlag := fs.Bool("no-cache", noCache, "Build without using layer cache")
	_ = fs.Parse(args)

	if err := requireDevcontainerCLI(); err != nil {
		return err
	}

	// In the devcontainers base-image repo: build named images or all.
	baseImages, _ := findBaseImages()
	if len(baseImages) > 0 {
		targets := fs.Args()
		if len(targets) == 0 {
			targets = baseImages
		}
		for _, img := range targets {
			ws := filepath.Join("images", img)
			if _, err := os.Stat(ws); err != nil {
				return fmt.Errorf("image %q not found — expected: images/%s/", img, img)
			}
			fmt.Printf("Building %s...\n", img)
			if err := devcontainerBuild(ws, *noCacheFlag); err != nil {
				return err
			}
		}
		return nil
	}

	// Standard project: build from CWD.
	if _, _, err := loadDevcontainerJSON(); err != nil {
		return err
	}
	return devcontainerBuild(".", *noCacheFlag)
}

func devcontainerBuild(workspaceFolder string, noCache bool) error {
	args := []string{"build", "--workspace-folder", workspaceFolder}
	if noCache {
		args = append(args, "--no-cache")
	}
	return runDevcontainerCLI(args...)
}

// ── Up ────────────────────────────────────────────────────────────────────────

func runDevUp(args []string) error {
	fs := flag.NewFlagSet("dev up", flag.ExitOnError)
	recreate := fs.Bool("recreate", false, "Remove existing container and recreate")
	_ = fs.Parse(args)

	if err := requireDevcontainerCLI(); err != nil {
		return err
	}
	if _, _, err := loadDevcontainerJSON(); err != nil {
		return err
	}

	cmdArgs := []string{"up", "--workspace-folder", "."}
	if *recreate {
		cmdArgs = append(cmdArgs, "--remove-existing-container")
	}
	return runDevcontainerCLI(cmdArgs...)
}

// ── Open ──────────────────────────────────────────────────────────────────────

func runDevOpen() error {
	if _, _, err := loadDevcontainerJSON(); err != nil {
		return err
	}
	cwd, _ := os.Getwd()
	for _, editor := range []string{"code", "cursor", "windsurf"} {
		if _, err := exec.LookPath(editor); err == nil {
			fmt.Printf("Opening in %s...\n", editor)
			cmd := exec.Command(editor, cwd)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
	}
	fmt.Printf("Open this folder in VS Code or Cursor — it will detect the devcontainer:\n  %s\n", cwd)
	return nil
}

// ── Exec ──────────────────────────────────────────────────────────────────────

func runDevExec(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: devx dev exec -- <command> [args...]")
	}
	if err := requireDevcontainerCLI(); err != nil {
		return err
	}
	if _, _, err := loadDevcontainerJSON(); err != nil {
		return err
	}
	cmdArgs := append([]string{"exec", "--workspace-folder", "."}, args...)
	return runDevcontainerCLI(cmdArgs...)
}

// ── devcontainer.json parsing ─────────────────────────────────────────────────

type devcontainerJSON struct {
	Name         string                     `json:"name"`
	Image        string                     `json:"image"`
	Build        *devcontainerBuildSpec     `json:"build"`
	Features     map[string]interface{}     `json:"features"`
	ForwardPorts []interface{}              `json:"forwardPorts"`
	Mounts       []interface{}              `json:"mounts"`
	RemoteEnv    map[string]string          `json:"remoteEnv"`

	PostCreateCommand interface{} `json:"postCreateCommand"`
	PostStartCommand  interface{} `json:"postStartCommand"`
	InitializeCommand interface{} `json:"initializeCommand"`

	Customizations map[string]json.RawMessage `json:"customizations"`
}

type devcontainerBuildSpec struct {
	Dockerfile string `json:"dockerfile"`
	Context    string `json:"context"`
}

// loadDevcontainerJSON finds and parses devcontainer.json from the current
// working directory, stripping // line comments before parsing.
func loadDevcontainerJSON() (*devcontainerJSON, string, error) {
	for _, p := range []string{
		filepath.Join(".devcontainer", "devcontainer.json"),
		"devcontainer.json",
	} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		stripped := stripJSONComments(data)
		var cfg devcontainerJSON
		if err := json.Unmarshal(stripped, &cfg); err != nil {
			return nil, p, fmt.Errorf("parsing %s: %w", p, err)
		}
		return &cfg, p, nil
	}
	cwd, _ := os.Getwd()
	return nil, "", fmt.Errorf(
		"no devcontainer.json found in %s\n  Expected: .devcontainer/devcontainer.json",
		cwd,
	)
}

// findBaseImages returns image names from images/<name>/devcontainer.json.
// Returns nil when not in the devcontainers base-image repo.
func findBaseImages() ([]string, error) {
	entries, err := os.ReadDir("images")
	if err != nil {
		return nil, err
	}
	var images []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join("images", e.Name(), "devcontainer.json")); err == nil {
			images = append(images, e.Name())
		}
	}
	return images, nil
}

// ── Display helpers ───────────────────────────────────────────────────────────

// shortFeatureName extracts just the name:tag from a ghcr.io feature reference.
// "ghcr.io/devcontainers/features/go:1" → "go:1"
func shortFeatureName(ref string) string {
	if idx := strings.LastIndex(ref, "/"); idx >= 0 {
		return ref[idx+1:]
	}
	return ref
}

// mountSummary returns a concise description of a mount entry.
func mountSummary(m interface{}) string {
	if s, ok := m.(string); ok {
		src := reCapture(s, `source=([^,]+)`)
		tgt := reCapture(s, `target=([^,]+)`)
		typ := reCapture(s, `type=([^,]+)`)
		if tgt != "" {
			if typ == "volume" && src != "" {
				return tgt + "  (volume: " + src + ")"
			}
			return tgt
		}
		return s
	}
	data, _ := json.Marshal(m)
	return string(data)
}

func reCapture(s, pattern string) string {
	m := regexp.MustCompile(pattern).FindStringSubmatch(s)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func lifecycleCommands(cfg *devcontainerJSON) []string {
	var out []string
	if s := commandString(cfg.InitializeCommand); s != "" {
		out = append(out, "initialize : "+truncate(s, 60))
	}
	if s := commandString(cfg.PostCreateCommand); s != "" {
		out = append(out, "postCreate : "+truncate(s, 60))
	}
	if s := commandString(cfg.PostStartCommand); s != "" {
		out = append(out, "postStart  : "+truncate(s, 60))
	}
	return out
}

func commandString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case []interface{}:
		parts := make([]string, len(t))
		for i, p := range t {
			parts[i] = fmt.Sprintf("%v", p)
		}
		return strings.Join(parts, " ")
	default:
		data, _ := json.Marshal(v)
		return string(data)
	}
}

func vscodeExtensions(cfg *devcontainerJSON) []string {
	raw, ok := cfg.Customizations["vscode"]
	if !ok {
		return nil
	}
	var vsc struct {
		Extensions []string `json:"extensions"`
	}
	if err := json.Unmarshal(raw, &vsc); err != nil {
		return nil
	}
	return vsc.Extensions
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// stripJSONComments removes // line comments so standard JSON parsing works
// on devcontainer.json files (which use JSONC format).
func stripJSONComments(data []byte) []byte {
	return regexp.MustCompile(`(?m)//[^\n]*`).ReplaceAll(data, nil)
}

// ── Devcontainer CLI runner ───────────────────────────────────────────────────

func requireDevcontainerCLI() error {
	if _, err := exec.LookPath("devcontainer"); err != nil {
		return fmt.Errorf(
			"devcontainer CLI not found\n" +
				"  Install with: npm install -g @devcontainers/cli",
		)
	}
	return nil
}

func runDevcontainerCLI(args ...string) error {
	cmd := exec.Command("devcontainer", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// ── Logs ──────────────────────────────────────────────────────────────────────

func runDevLogs(args []string) error {
	fs := flag.NewFlagSet("dev logs", flag.ExitOnError)
	follow := fs.Bool("follow", false, "Follow log output (like tail -f)")
	_ = fs.Parse(args)

	id, err := devcontainerID()
	if err != nil {
		return err
	}

	dockerArgs := []string{"logs", id}
	if *follow {
		dockerArgs = append(dockerArgs, "--follow")
	}
	cmd := exec.Command("docker", dockerArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// ── PS ────────────────────────────────────────────────────────────────────────

func runDevPS() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not found in PATH")
	}

	out, err := exec.Command("docker", "ps",
		"--filter", "label=devcontainer.local_folder",
		"--format", `{{.Names}}\t{{.Status}}\t{{.Label "devcontainer.local_folder"}}`,
	).Output()
	if err != nil {
		return fmt.Errorf("docker ps failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		fmt.Println("No devcontainers running.")
		return nil
	}

	fmt.Printf("%-40s  %-20s  %s\n", "Container", "Status", "Workspace")
	fmt.Println(strings.Repeat("─", 100))
	for _, line := range lines {
		parts := strings.SplitN(line, `\t`, 3)
		name, status, folder := "", "", ""
		if len(parts) > 0 {
			name = parts[0]
		}
		if len(parts) > 1 {
			status = parts[1]
		}
		if len(parts) > 2 {
			folder = parts[2]
		}
		// Shorten home dir.
		if home, _ := os.UserHomeDir(); home != "" {
			folder = strings.Replace(folder, home, "~", 1)
		}
		fmt.Printf("%-40s  %-20s  %s\n", name, status, folder)
	}
	return nil
}

// ── Shell ─────────────────────────────────────────────────────────────────────

func runDevShell() error {
	if err := requireDevcontainerCLI(); err != nil {
		return err
	}
	if _, _, err := loadDevcontainerJSON(); err != nil {
		return err
	}
	// Try bash first, fall back to sh.
	for _, shell := range []string{"/bin/bash", "/bin/sh"} {
		args := []string{"exec", "--workspace-folder", ".", "--", shell}
		cmd := exec.Command("devcontainer", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("could not start shell in devcontainer")
}

// devcontainerID returns the Docker container ID for the current workspace.
func devcontainerID() (string, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return "", fmt.Errorf("docker not found in PATH")
	}
	abs, err := filepath.Abs(".")
	if err != nil {
		return "", err
	}
	out, err := exec.Command("docker", "ps",
		"--filter", "label=devcontainer.local_folder="+abs,
		"--format", "{{.ID}}",
	).Output()
	if err != nil {
		return "", fmt.Errorf("docker ps failed: %w", err)
	}
	id := strings.TrimSpace(string(out))
	if id == "" {
		return "", fmt.Errorf("no devcontainer running for %s\n  Start it with: devx dev up", abs)
	}
	return strings.SplitN(id, "\n", 2)[0], nil
}

// ── Usage ─────────────────────────────────────────────────────────────────────

func printDevUsage() {
	fmt.Println("devx dev - build and manage devcontainers")
	fmt.Println()
	fmt.Println("Reads .devcontainer/devcontainer.json from the current directory.")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  devx dev                         show config, image, features, mounts")
	fmt.Println("  devx dev build [image]           build the devcontainer image")
	fmt.Println("    --no-cache                     skip layer cache")
	fmt.Println("  devx dev rebuild [image]         build without cache (shortcut)")
	fmt.Println("  devx dev up [--recreate]         start the devcontainer")
	fmt.Println("  devx dev open                    open in VS Code / Cursor")
	fmt.Println("  devx dev exec -- <cmd> [args]    run a command inside the container")
	fmt.Println("  devx dev shell                   open an interactive shell in the container")
	fmt.Println("  devx dev logs [--follow]         stream container logs")
	fmt.Println("  devx dev ps                      list all running devcontainers on this host")
	fmt.Println()
	fmt.Println("In a devcontainers base-image repo (images/<name>/devcontainer.json):")
	fmt.Println("  devx dev build go-dev            build a single image")
	fmt.Println("  devx dev build                   build all images")
}
