package main

import (
	"context"
	"fmt"
	"os"
)

// version is set at build time via -ldflags "-X main.version=v1.2.3"
var version = "dev"

const (
	manifestFile = "devx.yaml"
	devxDir      = ".devx"
	composeFile  = "compose.yaml"
	k8sFile      = "k8s.yaml"
	stateFile    = "state.json"
	lockFile     = "devx.lock"
)

type state struct {
	Profile   string `json:"profile"`
	Runtime   string `json:"runtime"`
	Telemetry bool   `json:"telemetry"`
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	ctx := context.Background()
	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "init":
		err = runInit(args)
	case "up":
		err = runUp(ctx, args)
	case "down":
		err = runDown(ctx, args)
	case "status":
		err = runStatus(ctx, args)
	case "logs":
		err = runLogs(ctx, args)
	case "exec":
		err = runExec(ctx, args)
	case "doctor":
		err = runDoctor(ctx, args)
	case "setup":
		err = runSetup(ctx, args)
	case "validate":
		err = runValidate(args)
	case "render":
		err = runRender(ctx, args)
	case "lock":
		err = runLock(ctx, args)
	case "providers":
		err = runProviders(ctx, args)
	case "export":
		err = runExport(ctx, args)
	case "ai":
		err = runAI(ctx, args)
	case "dev":
		err = runDev(ctx, args)
	case "run":
		err = runRun(ctx, args)
	case "clone":
		err = runClone(ctx, args)
	case "update":
		err = runUpdate(ctx, args)
	case "completion":
		err = runCompletion(args)
	case "ci":
		err = runCI(ctx, args)
	case "env":
		err = runEnv(ctx, args)
	case "auth":
		err = runAuth(ctx, args)
	case "new":
		err = runNew(ctx, args)
	case "version", "--version", "-v":
		fmt.Println("devx " + version)
		return
	case "help", "-h", "--help":
		printUsage()
		return
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("devx - cross-platform dev orchestrator")
	fmt.Println("\nUsage:")
	fmt.Println("  devx init                          interactive project scaffold (devx.yaml + devcontainer)")
	fmt.Println("  devx setup [--fix] [--json]")
	fmt.Println("  devx run [script]                  run a script from devx.yaml")
	fmt.Println("  devx clone <repo> [dir]            clone repo and run devx setup")
	fmt.Println("  devx up [--profile local|ci|k8s] [--build] [--pull]")
	fmt.Println("  devx down [--volumes]")
	fmt.Println("  devx status [--json]               project + devcontainer + AI status")
	fmt.Println("  devx logs [service] [--follow] [--since 10m] [--json]")
	fmt.Println("  devx exec <service> -- <cmd...>")
	fmt.Println("  devx doctor [--fix] [--json]       health-check all tools, backends, and config")
	fmt.Println("  devx validate [--file path]")
	fmt.Println("  devx render compose|k8s [--write]")
	fmt.Println("  devx lock update")
	fmt.Println("  devx providers install|list")
	fmt.Println("  devx export --format compose|k8s|helm|terraform [--profile name]")
	fmt.Println("  devx ai                            AI setup wizard or launch configured tool")
	fmt.Println("  devx ai chat [question]            one-off AI question")
	fmt.Println("  devx ai setup|status|reset")
	fmt.Println("  devx ai model list|pull|remove")
	fmt.Println("  devx dev                           devcontainer status")
	fmt.Println("  devx dev build|rebuild [image] [--no-cache]")
	fmt.Println("  devx dev up [--recreate]")
	fmt.Println("  devx dev open|exec|shell")
	fmt.Println("  devx dev logs [--follow]")
	fmt.Println("  devx dev ps")
	fmt.Println("  devx env [--show]                  show/validate env vars from devcontainer + .env")
	fmt.Println("  devx auth                          check auth status for GitHub, Anthropic, OpenAI")
	fmt.Println("  devx new service|dep               add a service or dependency to devx.yaml")
	fmt.Println("  devx ci [--write] [--stack dotnet|node|go|python]")
	fmt.Println("  devx ci run [--job <name>]         run CI locally with act")
	fmt.Println("  devx update [--check]              update devx to latest version")
	fmt.Println("  devx completion bash|zsh|fish      generate shell completions")
	fmt.Println("  devx version")
}
