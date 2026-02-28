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
	case "render":
		err = runRender(ctx, args)
	case "lock":
		err = runLock(ctx, args)
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
	fmt.Println("  devx init")
	fmt.Println("  devx up [--profile local|ci|k8s] [--build] [--pull] [--no-telemetry]")
	fmt.Println("  devx down [--volumes]")
	fmt.Println("  devx status")
	fmt.Println("  devx logs [service] [--follow] [--since 10m] [--json]")
	fmt.Println("  devx exec <service> -- <cmd...>")
	fmt.Println("  devx doctor [--fix]")
	fmt.Println("  devx render compose [--write] [--no-telemetry]")
	fmt.Println("  devx render k8s [--profile name] [--namespace ns] [--write]")
	fmt.Println("  devx lock update")
	fmt.Println("  devx version")
}
