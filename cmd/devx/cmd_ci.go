package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

func runCI(_ context.Context, args []string) error {
	if len(args) > 0 && args[0] == "run" {
		return runCILocal(args[1:])
	}

	fs := flag.NewFlagSet("ci", flag.ExitOnError)
	write := fs.Bool("write", false, "Write .github/workflows/ci.yml (default: print to stdout)")
	stack := fs.String("stack", "", "Override stack detection: dotnet|node|go|python")
	_ = fs.Parse(args)

	detectedStack := *stack
	if detectedStack == "" {
		detectedStack = detectStack()
	}

	workflow := generateCIWorkflow(detectedStack)

	if *write {
		if err := os.MkdirAll(".github/workflows", 0755); err != nil {
			return err
		}
		path := ".github/workflows/ci.yml"
		if err := os.WriteFile(path, []byte(workflow), 0644); err != nil {
			return err
		}
		fmt.Printf("✓  wrote %s  (stack: %s)\n", path, detectedStack)
		fmt.Println()
		fmt.Println("Commit and push to activate CI:")
		fmt.Println("  git add .github/workflows/ci.yml && git commit -m 'ci: add GitHub Actions workflow'")
		return nil
	}

	fmt.Printf("# Generated for stack: %s\n", detectedStack)
	fmt.Println("# Run with --write to save to .github/workflows/ci.yml")
	fmt.Println()
	fmt.Print(workflow)
	return nil
}

// detectStack inspects the project and returns the best-guess stack name.
func detectStack() string {
	// 1. Check devcontainer.json features.
	for _, path := range []string{".devcontainer/devcontainer.json", "devcontainer.json"} {
		if data, err := readJSONC(path); err == nil {
			if stack := stackFromDevcontainerFeatures(data); stack != "" {
				return stack
			}
		}
	}

	// 2. Check well-known files.
	switch {
	case fileExists("*.sln") || fileExists("*.csproj") || globExists("*.sln") || globExists("*.csproj"):
		return "dotnet"
	case fileExists("package.json"):
		return "node"
	case fileExists("go.mod"):
		return "go"
	case fileExists("pyproject.toml") || fileExists("requirements.txt") || fileExists("setup.py"):
		return "python"
	}

	return "other"
}

func stackFromDevcontainerFeatures(raw []byte) string {
	var dc struct {
		Image    string                     `json:"image"`
		Features map[string]json.RawMessage `json:"features"`
	}
	if err := json.Unmarshal(raw, &dc); err != nil {
		return ""
	}
	img := strings.ToLower(dc.Image)
	for k := range dc.Features {
		k = strings.ToLower(k)
		switch {
		case strings.Contains(k, "dotnet") || strings.Contains(img, "dotnet"):
			return "dotnet"
		case strings.Contains(k, "node") || strings.Contains(img, "node") || strings.Contains(img, "javascript"):
			return "node"
		case strings.Contains(k, "/go") || strings.Contains(img, "golang"):
			return "go"
		case strings.Contains(k, "python") || strings.Contains(img, "python"):
			return "python"
		}
	}
	if strings.Contains(img, "dotnet") {
		return "dotnet"
	}
	return ""
}

// globExists returns true if any file matches a simple glob (no path sep).
func globExists(pattern string) bool {
	entries, err := os.ReadDir(".")
	if err != nil {
		return false
	}
	ext := strings.TrimPrefix(pattern, "*")
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ext) {
			return true
		}
	}
	return false
}

// readJSONC reads a file and strips // comments for JSON parsing.
func readJSONC(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`(?m)//[^\n]*`)
	return re.ReplaceAll(data, nil), nil
}

// generateCIWorkflow returns a GitHub Actions workflow YAML for the given stack.
func generateCIWorkflow(stack string) string {
	switch stack {
	case "dotnet":
		return dotnetWorkflow()
	case "node":
		return nodeWorkflow()
	case "go":
		return goWorkflow()
	case "python":
		return pythonWorkflow()
	default:
		return genericWorkflow()
	}
}

func dotnetWorkflow() string {
	return `name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup .NET
        uses: actions/setup-dotnet@v4
        with:
          dotnet-version: '8.x'

      - name: Restore dependencies
        run: dotnet restore

      - name: Build
        run: dotnet build --no-restore --configuration Release

      - name: Test
        run: dotnet test --no-build --configuration Release --logger "trx;LogFileName=results.trx"

      - name: Upload test results
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: test-results
          path: "**/*.trx"
`
}

func nodeWorkflow() string {
	return `name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'

      - name: Install dependencies
        run: npm ci

      - name: Build
        run: npm run build

      - name: Test
        run: npm test -- --passWithNoTests
`
}

func goWorkflow() string {
	return `name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version-file: 'go.mod'

      - name: Build
        run: go build ./...

      - name: Test
        run: go test ./...

      - name: Vet
        run: go vet ./...
`
}

func pythonWorkflow() string {
	return `name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Python
        uses: actions/setup-python@v5
        with:
          python-version: '3.12'

      - name: Install dependencies
        run: |
          python -m pip install --upgrade pip
          pip install -r requirements.txt

      - name: Test
        run: pytest --tb=short
`
}

func genericWorkflow() string {
	return `name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Build
        run: echo "Add your build command here"

      - name: Test
        run: echo "Add your test command here"
`
}

// ── Local CI run ──────────────────────────────────────────────────────────────

func runCILocal(args []string) error {
	fs := flag.NewFlagSet("ci run", flag.ExitOnError)
	job := fs.String("job", "", "Run a specific job (e.g. --job build)")
	_ = fs.Parse(args)

	if _, err := exec.LookPath("act"); err != nil {
		fmt.Println("'act' is not installed. act lets you run GitHub Actions locally.")
		fmt.Println()
		fmt.Println("Install with:")
		fmt.Println("  brew install act           (macOS)")
		fmt.Println("  choco install act-cli      (Windows)")
		fmt.Println("  curl -s https://raw.githubusercontent.com/nektos/act/master/install.sh | bash")
		fmt.Println()
		fmt.Println("Then re-run: devx ci run")
		return nil
	}

	actArgs := []string{}
	if *job != "" {
		actArgs = append(actArgs, "--job", *job)
	}

	fmt.Println("Running CI locally with act...")
	if *job != "" {
		fmt.Printf("  Job: %s\n", *job)
	}
	fmt.Println()

	cmd := exec.Command("act", actArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
