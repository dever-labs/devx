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
)

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
		err = runInit(ctx, args)
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
	case "validate":
		err = runValidate(args)
	case "tasks":
		err = runTasks(args)
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
		err = runNew(args)
	case "version", "--version", "-v":
		fmt.Println("devx " + version)
		return
	case "help", "-h", "--help":
		printUsage()
		return
	default:
		// `devx <name>` is shorthand for `devx run <name>` — reaching
		// here already means <name> isn't a built-in command, since
		// those are all handled above and always take precedence.
		err = runRun(ctx, os.Args[1:])
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("devx - cross-platform dev task orchestrator")
	fmt.Println("\nUsage:")
	fmt.Println("  devx init [--fix] [--json]          interactive scaffold for a new project; always leaves it ready to run")
	fmt.Println("  devx up [<name...>] [-d|--detach] [--dry-run] [--keep-going] [--group <name>]")
	fmt.Println("                                      bring up the task graph (or a partial closure)")
	fmt.Println("  devx run <name...> [--dry-run] [--watch]")
	fmt.Println("                                      run task(s) directly, respecting deps")
	fmt.Println("  devx <name>                         shorthand for 'devx run <name>'")
	fmt.Println("  devx down [--force]                stop every running detached task")
	fmt.Println("  devx status [--json]               task + project + devcontainer + AI status")
	fmt.Println("  devx logs [name] [--follow]         tail captured output for one or all running tasks")
	fmt.Println("  devx exec <name> -- <cmd...>        run a command inside a running task's container")
	fmt.Println("  devx tasks [--json]                 dump the full resolved task list")
	fmt.Println("  devx doctor [--fix] [--json]        health-check tools, requires:, and port collisions")
	fmt.Println("  devx validate [--file path] [--json]")
	fmt.Println("  devx ai                             AI setup wizard or launch configured tool")
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
	fmt.Println("  devx new <name> [--run <cmd>]      append a minimal task skeleton to devx.yaml")
	fmt.Println("  devx ci [--write] [--stack dotnet|node|go|python]")
	fmt.Println("  devx ci run [--job <name>]         run CI locally with act")
	fmt.Println("  devx update [--check]              update devx to latest version")
	fmt.Println("  devx completion bash|zsh|fish      generate shell completions")
	fmt.Println("  devx version")
	fmt.Println("\nRemoved in the task-based devx.yaml model (see docs/design/task-executor-refactor.md):")
	fmt.Println("  devx render, devx lock, devx providers, devx export — run: is executed directly, never rendered to compose/k8s/helm/terraform.")
}
