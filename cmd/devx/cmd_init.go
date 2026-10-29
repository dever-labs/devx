package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dever-labs/devx/internal/detect"
)

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	_ = fs.Parse(args)

	if fileExists(manifestFile) {
		return fmt.Errorf("%s already exists — run 'devx ai reset' to reconfigure AI, or delete devx.yaml to start over", manifestFile)
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("devx init — set up this project")
	fmt.Println()

	// Project name — default to current directory name.
	cwd, _ := os.Getwd()
	defaultName := filepath.Base(cwd)
	projectName, err := promptString(reader, "Project name", defaultName)
	if err != nil {
		return err
	}
	fmt.Println()

	// Stack selection.
	stackIdx, err := promptChoice(reader, "Primary language / stack?",
		[]string{
			"dotnet   — C#, F#, ASP.NET Core",
			"node     — TypeScript, JavaScript, Next.js",
			"go       — Go",
			"python   — Python, FastAPI, Django",
			"other    — write devx.yaml manually",
		}, 0)
	if err != nil {
		return err
	}
	stacks := []string{"dotnet", "node", "go", "python", "other"}
	stack := stacks[stackIdx]
	fmt.Println()

	// Devcontainer?
	wantDevcontainer, err := promptYN(reader, "Set up a devcontainer?", true)
	if err != nil {
		return err
	}
	fmt.Println()

	// AI setup?
	wantAI, err := promptYN(reader, "Add AI configuration (devx ai)?", true)
	if err != nil {
		return err
	}
	fmt.Println()

	// Write devx.yaml
	if err := writeInitManifest(projectName, stack, wantAI); err != nil {
		return err
	}
	fmt.Printf("✓  wrote %s\n", manifestFile)

	// Write .devcontainer/devcontainer.json
	if wantDevcontainer {
		if err := writeDevcontainer(stack); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not write devcontainer.json: %v\n", err)
		} else {
			fmt.Println("✓  wrote .devcontainer/devcontainer.json")
		}
	}

	if err := ensureDevxDir(); err != nil {
		return err
	}
	if err := ensureGitignore(); err != nil {
		return err
	}
	fmt.Println("✓  updated .gitignore")
	fmt.Println()

	if wantAI {
		fmt.Println("Run  devx ai  to configure your AI tool.")
	}
	if wantDevcontainer {
		fmt.Println("Run  devx dev up  to start the devcontainer.")
	}
	fmt.Println("Run  devx doctor  to verify your environment.")
	return nil
}

func writeInitManifest(projectName, stack string, withAI bool) error {
	var sb strings.Builder

	sb.WriteString("# yaml-language-server: $schema=https://raw.githubusercontent.com/dever-labs/devx/main/schemas/devx.schema.json\n")
	sb.WriteString("version: 1\n\nproject:\n")
	fmt.Fprintf(&sb, "  name: %s\n", projectName)

	if withAI {
		sb.WriteString("\n# AI configuration — run 'devx ai' to set up\n")
		sb.WriteString("ai:\n  provider: local\n  local:\n    backend: auto\n")
	}

	var mgr string
	if stack == "node" {
		mgr = "npm"
		if ws, ok := detect.JS("."); ok {
			mgr = ws.Manager
			note := "single package"
			if ws.IsWorkspace {
				note = "workspace root"
			}
			fmt.Fprintf(&sb, "\n# Detected %s (%s) from package.json — installs once at the\n# workspace root instead of per-package setup: steps.\n", mgr, note)
			fmt.Fprintf(&sb, "setup:\n  - name: install dependencies\n    run: %s\n    runOnce: true\n", ws.InstallCommand())
		}
	}

	sb.WriteString("\n# Tasks runnable with: devx run <name> (or devx <name>)\n")
	switch stack {
	case "dotnet":
		sb.WriteString("build:\n  desc: Build the solution\n  run: dotnet build\n\n")
		sb.WriteString("test:\n  desc: Run all tests\n  run: dotnet test --logger \"console;verbosity=minimal\"\n")
	case "node":
		runPrefix := mgr + " run" // "npm run <script>" / "pnpm run <script>"
		if mgr == "yarn" {
			runPrefix = "yarn" // yarn doesn't need the "run" subcommand for package.json scripts
		}
		// "test"/"start" are reserved lifecycle script names every
		// manager runs directly without "run" (npm test, pnpm test,
		// yarn test all work identically).
		fmt.Fprintf(&sb, "build:\n  desc: Build\n  run: %s build\n\n", runPrefix)
		fmt.Fprintf(&sb, "test:\n  desc: Run tests\n  run: %s test\n\n", mgr)
		fmt.Fprintf(&sb, "dev:\n  desc: Start dev server\n  run: %s dev\n", runPrefix)
	case "go":
		sb.WriteString("build:\n  desc: Build all packages\n  run: go build ./...\n\n")
		sb.WriteString("test:\n  desc: Run all tests\n  run: go test ./...\n")
	case "python":
		sb.WriteString("test:\n  desc: Run tests\n  run: pytest\n\n")
		sb.WriteString("lint:\n  desc: Lint\n  run: ruff check .\n")
	default:
		sb.WriteString("build:\n  desc: Build\n  run: echo 'add your build command here'\n")
	}

	return os.WriteFile(manifestFile, []byte(sb.String()), 0644)
}

func writeDevcontainer(stack string) error {
	if err := os.MkdirAll(".devcontainer", 0755); err != nil {
		return err
	}
	if fileExists(".devcontainer/devcontainer.json") {
		return nil // don't overwrite
	}

	// Base image per stack from dever-labs/devcontainers.
	imageMap := map[string]string{
		"dotnet": "ghcr.io/dever-labs/devcontainers/dotnet-dev:latest",
		"node":   "ghcr.io/dever-labs/devcontainers/frontend-dev:latest",
		"go":     "ghcr.io/dever-labs/devcontainers/base:latest",
		"python": "ghcr.io/dever-labs/devcontainers/base:latest",
		"other":  "mcr.microsoft.com/devcontainers/base:ubuntu-24.04",
	}
	image, ok := imageMap[stack]
	if !ok {
		image = "mcr.microsoft.com/devcontainers/base:ubuntu-24.04"
	}

	content := fmt.Sprintf(`{
  "name": "Dev Container",
  "image": "%s",

  // Runs on the HOST before the container starts.
  // Detects platform and sets up the right AI backend (MLX or Ollama).
  "initializeCommand": "curl -fsSL https://raw.githubusercontent.com/dever-labs/devcontainers/main/scripts/ai-setup | bash",

  "features": {
    "ghcr.io/dever-labs/devcontainers/features/local-ai:1": {}
  },

  "customizations": {
    "vscode": {
      "extensions": []
    }
  }
}
`, image)

	return os.WriteFile(".devcontainer/devcontainer.json", []byte(content), 0644)
}

