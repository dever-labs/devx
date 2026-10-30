package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dever-labs/devx/internal/detect"
)

// runInit is the single entry point for both bootstrapping a new project
// (the interactive wizard, when devx.yaml doesn't exist yet) and preparing
// an existing one (tool checks, setup steps, envFiles — what used to be
// the separate `devx setup` command). Folding "setup" into "init" means
// there's one command to remember: `devx init` always leaves the project
// ready to run, whether or not devx.yaml already existed.
func runInit(ctx context.Context, args []string) error {
	if !fileExists(manifestFile) {
		if err := runInitWizard(); err != nil {
			return err
		}
		fmt.Println()
	}
	// --fix/--json/--no-prompt (if present in args) are parsed here, by
	// the tool/setup-steps/envFiles execution that used to be `devx setup`.
	return runProjectSetup(ctx, args)
}

func runInitWizard() error {
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

	// VS Code attach-debugging config?
	wantLaunchJSON, err := promptYN(reader, "Generate .vscode/launch.json for attach-based debugging?", stack == "node" || stack == "go")
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

	// Write .vscode/launch.json
	if wantLaunchJSON {
		ok, err := writeLaunchJSON(stack)
		switch {
		case err != nil:
			fmt.Fprintf(os.Stderr, "warning: could not write launch.json: %v\n", err)
		case ok:
			fmt.Println("✓  wrote .vscode/launch.json")
		default:
			fmt.Printf("ℹ  attach-debugging launch.json isn't templated for %q yet — skipped\n", stack)
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
	if wantLaunchJSON {
		switch stack {
		case "node":
			fmt.Println("Add --inspect (or --inspect-brk) to a task's run: command, start it with devx up, then press F5 to attach.")
		case "go":
			fmt.Println("Wrap a task's run: command with dlv exec --headless --listen=:2345 --api-version=2, start it with devx up, then press F5 to attach.")
		}
	}
	fmt.Println("Checking tools and running setup now — re-run  devx init --fix  any time to repeat.")
	return nil
}

// writeLaunchJSON writes a stack-specific .vscode/launch.json attach
// configuration for interactive debugging of a devx-managed process
// task. Returns ok=false (no error) for stacks without a templated
// debug adapter yet, so callers can report "skipped" rather than fail
// init over an optional, best-effort convenience file.
func writeLaunchJSON(stack string) (bool, error) {
	var content string
	switch stack {
	case "node":
		content = `{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "devx: attach (Node inspector)",
      "type": "node",
      "request": "attach",
      "port": 9229,
      "restart": true,
      "skipFiles": ["<node_internals>/**"]
    }
  ]
}
`
	case "go":
		content = `{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "devx: attach (Delve)",
      "type": "go",
      "request": "attach",
      "mode": "remote",
      "port": 2345,
      "host": "127.0.0.1"
    }
  ]
}
`
	default:
		return false, nil
	}

	if err := os.MkdirAll(".vscode", 0755); err != nil {
		return false, err
	}
	if fileExists(".vscode/launch.json") {
		return false, nil // don't overwrite
	}
	if err := os.WriteFile(".vscode/launch.json", []byte(content), 0644); err != nil {
		return false, err
	}
	return true, nil
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
		var toolLines []string

		if v, ok := detect.NodeVersion("."); ok {
			var winCmd, macCmd, linCmd string
			if m, ok := detect.NodeVersionManager(); ok {
				cmd := detect.NodeInstallCommand(m, v)
				winCmd, macCmd, linCmd = cmd, cmd, cmd
				fmt.Fprintf(&sb, "\n# Detected node %s from the project and %s installed locally —\n# installs pin to this version through %s instead of the OS package manager.\n", v, m, m)
			} else {
				winCmd = "choco install nodejs --version=" + v
				macCmd = "brew install node@" + v
				linCmd = "sudo apt-get install -y nodejs=" + v + "*"
				fmt.Fprintf(&sb, "\n# Detected node %s from the project. No nvm/fnm/volta found locally,\n# so install falls back to the OS package manager — install one of\n# those tools for version-manager-aware installs instead.\n", v)
			}
			toolLines = append(toolLines, fmt.Sprintf("  - name: node\n    version: %s\n    check: node --version\n    install:\n      windows: %s\n      macos: %s\n      linux: %s\n", v, winCmd, macCmd, linCmd))
		}

		ws, wsOK := detect.JS(".")
		if wsOK {
			mgr = ws.Manager
			if ws.ManagerVersion != "" {
				if cmd, ok := detect.CorepackInstallCommand(ws.ManagerVersion); ok {
					toolLines = append(toolLines, fmt.Sprintf("  - name: %s\n    version: %s\n    check: %s --version\n    install:\n      windows: %s\n      macos: %s\n      linux: %s\n", mgr, ws.ManagerVersion, mgr, cmd, cmd, cmd))
				}
			}
		}

		if len(toolLines) > 0 {
			sb.WriteString("tools:\n")
			for _, l := range toolLines {
				sb.WriteString(l)
			}
		}

		if wsOK {
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
