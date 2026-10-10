package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"

	"github.com/dever-labs/devx/internal/k8s"
	devxruntime "github.com/dever-labs/devx/internal/runtime"
)

func runDown(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("down", flag.ExitOnError)
	volumes := fs.Bool("volumes", false, "Remove volumes")
	_ = fs.Parse(args)

	manifest, profName, prof, err := loadProfile("")
	if err != nil {
		return err
	}

	if profileRuntime(prof) == "k8s" {
		if _, err := selectRuntime(ctx); err != nil {
			return err
		}
		return runDownK8s(ctx)
	}

	var rt devxruntime.Runtime
	if prof.HasContainers() {
		rt, err = selectRuntime(ctx)
		if err != nil {
			return err
		}
	}

	enableTelemetry := telemetryFromState()

	composePath := filepath.Join(devxDir, composeFile)
	if rt != nil && !fileExists(composePath) {
		if err := ensureDevxDir(); err != nil {
			return err
		}
		if err := writeCompose(composePath, manifest, profName, prof, nil, enableTelemetry); err != nil {
			return err
		}
	}

	if len(prof.Hooks.BeforeDown) > 0 {
		for _, h := range prof.Hooks.BeforeDown {
			if h.Exec != "" && rt == nil {
				return fmt.Errorf("beforeDown exec hook needs a container service, but profile %q has none; use a run hook", profName)
			}
		}
		fmt.Println("Running beforeDown hooks...")
		if _, err := runHooks(ctx, rt, composePath, manifest.Project.Name, prof.Hooks.BeforeDown); err != nil {
			return err
		}
	}

	procErr := stopProcesses()
	if rt != nil {
		if err := rt.Down(ctx, composePath, manifest.Project.Name, *volumes); err != nil {
			return err
		}
	}
	return procErr
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
