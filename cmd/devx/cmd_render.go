package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dever-labs/devx/internal/k8s"
	"github.com/dever-labs/devx/internal/lock"
)

func runRender(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "compose" {
		return runRenderK8s(ctx, args)
	}

	fs := flag.NewFlagSet("render", flag.ExitOnError)
	write := fs.Bool("write", false, "Write to .devx/compose.yaml")
	telemetry := fs.Bool("telemetry", false, "Include the built-in observability stack")
	noTelemetry := fs.Bool("no-telemetry", false, "Deprecated: telemetry is off by default")
	_ = fs.Parse(args[1:])

	manifest, profName, prof, err := loadProfile("")
	if err != nil {
		return err
	}

	lockfile, _ := lock.Load(lockFile)

	composed, err := buildCompose(manifest, profName, prof, lockfile, *telemetry && !*noTelemetry)
	if err != nil {
		return err
	}

	if *write {
		if err := ensureDevxDir(); err != nil {
			return err
		}
		composePath := filepath.Join(devxDir, composeFile)
		return writeCompose(composePath, manifest, profName, prof, lockfile, *telemetry && !*noTelemetry)
	}

	fmt.Print(composed)
	return nil
}

func runRenderK8s(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "k8s" {
		return errors.New("render requires 'compose' or 'k8s'")
	}

	fs := flag.NewFlagSet("render-k8s", flag.ExitOnError)
	profile := fs.String("profile", "", "Profile to use")
	namespace := fs.String("namespace", "", "Kubernetes namespace")
	write := fs.Bool("write", false, "Write to .devx/k8s.yaml")
	_ = fs.Parse(args[1:])

	manifest, profName, prof, err := loadProfile(*profile)
	if err != nil {
		return err
	}

	output, err := k8s.Render(manifest, profName, prof, *namespace)
	if err != nil {
		return err
	}

	if *write {
		if err := ensureDevxDir(); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(devxDir, k8sFile), []byte(output), 0600)
	}

	fmt.Print(output)
	return nil
}
