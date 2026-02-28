package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"

	"github.com/dever-labs/devx/internal/k8s"
)

func runDown(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("down", flag.ExitOnError)
	volumes := fs.Bool("volumes", false, "Remove volumes")
	_ = fs.Parse(args)

	manifest, profName, prof, err := loadProfile("")
	if err != nil {
		return err
	}

	rt, err := selectRuntime(ctx)
	if err != nil {
		return err
	}

	enableTelemetry := telemetryFromState()
	runtimeMode := profileRuntime(prof)
	if runtimeMode == "k8s" {
		return runDownK8s(ctx)
	}

	composePath := filepath.Join(devxDir, composeFile)
	if !fileExists(composePath) {
		if err := ensureDevxDir(); err != nil {
			return err
		}
		if err := writeCompose(composePath, manifest, profName, prof, nil, enableTelemetry); err != nil {
			return err
		}
	}

	if len(prof.Hooks.BeforeDown) > 0 {
		fmt.Println("Running beforeDown hooks...")
		if err := runHooks(ctx, rt, composePath, manifest.Project.Name, prof.Hooks.BeforeDown); err != nil {
			return err
		}
	}

	return rt.Down(ctx, composePath, manifest.Project.Name, *volumes)
}

func runDownK8s(ctx context.Context) error {
	path := filepath.Join(devxDir, k8sFile)
	if !fileExists(path) {
		return fmt.Errorf("%s not found; run 'devx render k8s --write' or 'devx up --profile <k8s profile>'", path)
	}
	if err := k8s.Delete(ctx, path); err != nil {
		return err
	}
	fmt.Println("Kubernetes resources deleted")
	return nil
}
