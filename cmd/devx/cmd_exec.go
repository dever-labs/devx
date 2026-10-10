package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/dever-labs/devx/internal/process"
)

func runExec(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("exec requires a service name")
	}

	sep := -1
	for i, arg := range args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep == -1 || sep == len(args)-1 {
		return errors.New("exec requires a command after --")
	}

	service := args[0]
	cmdArgs := args[sep+1:]

	manifest, profName, prof, err := loadProfile("")
	if err != nil {
		return err
	}

	if profileRuntime(prof) == "k8s" {
		return errors.New("exec for k8s runtime is not supported yet")
	}

	if svc, ok := prof.Services[service]; ok && svc.IsProcess() {
		resolved := resolveProfile(manifest, prof)
		code, err := process.Exec(ctx, processSpec(service, resolved.Services[service]), cmdArgs)
		if err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("exec exited with code %d", code)
		}
		return nil
	}

	rt, err := selectRuntime(ctx)
	if err != nil {
		return err
	}

	enableTelemetry := telemetryFromState()
	composePath := filepath.Join(devxDir, composeFile)
	if err := ensureDevxDir(); err != nil {
		return err
	}
	if err := writeCompose(composePath, manifest, profName, prof, nil, enableTelemetry); err != nil {
		return err
	}

	code, err := rt.Exec(ctx, composePath, manifest.Project.Name, service, cmdArgs)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("exec exited with code %d", code)
	}
	return nil
}
