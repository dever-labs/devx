package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	sb.WriteString("version: 1\n\nproject:\n")
	sb.WriteString(fmt.Sprintf("  name: %s\n", projectName))
	sb.WriteString("  defaultProfile: local\n")

	if withAI {
		sb.WriteString("\n# AI configuration — run 'devx ai' to set up\n")
		sb.WriteString("ai:\n  provider: local\n  local:\n    backend: auto\n")
	}

	sb.WriteString("\n# Scripts runnable with: devx run <name>\n")
	sb.WriteString("scripts:\n")
	switch stack {
	case "dotnet":
		sb.WriteString("  - name: build\n    run: dotnet build\n    desc: Build the solution\n")
		sb.WriteString("  - name: test\n    run: dotnet test --logger \"console;verbosity=minimal\"\n    desc: Run all tests\n")
	case "node":
		sb.WriteString("  - name: build\n    run: npm run build\n    desc: Build\n")
		sb.WriteString("  - name: test\n    run: npm test\n    desc: Run tests\n")
		sb.WriteString("  - name: dev\n    run: npm run dev\n    desc: Start dev server\n")
	case "go":
		sb.WriteString("  - name: build\n    run: go build ./...\n    desc: Build all packages\n")
		sb.WriteString("  - name: test\n    run: go test ./...\n    desc: Run all tests\n")
	case "python":
		sb.WriteString("  - name: test\n    run: pytest\n    desc: Run tests\n")
		sb.WriteString("  - name: lint\n    run: ruff check .\n    desc: Lint\n")
	default:
		sb.WriteString("  - name: build\n    run: echo 'add your build command here'\n    desc: Build\n")
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

