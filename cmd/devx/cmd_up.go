package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dever-labs/devx/internal/config"
	"github.com/dever-labs/devx/internal/k8s"
	"github.com/dever-labs/devx/internal/lock"
	"github.com/dever-labs/devx/internal/process"
	"github.com/dever-labs/devx/internal/runtime"
)

func runUp(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("up", flag.ExitOnError)
	profile := fs.String("profile", "", "Profile to use")
	build := fs.Bool("build", false, "Build images")
	pull := fs.Bool("pull", false, "Always pull images")
	telemetry := fs.Bool("telemetry", false, "Enable the built-in observability stack")
	noTelemetry := fs.Bool("no-telemetry", false, "Deprecated: telemetry is off by default")
	attach := fs.String("attach", "", "Run this process service in the foreground with stdio attached (debuggers, interactive tools)")
	_ = fs.Parse(args)

	manifest, profName, prof, err := loadProfile(*profile)
	if err != nil {
		return err
	}

	if *attach != "" {
		if svc, ok := prof.Services[*attach]; !ok || !svc.IsProcess() {
			return fmt.Errorf("--attach: %q is not a process service in profile %q", *attach, profName)
		}
	}

	runtimeMode := profileRuntime(prof)
	if runtimeMode == "k8s" {
		if _, err := selectRuntime(ctx); err != nil {
			return err
		}
		ensureProviders(manifest, prof)
		if err := ensureDevxDir(); err != nil {
			return err
		}
		return runUpK8s(ctx, manifest, profName, prof)
	}

	ensureProviders(manifest, prof)
	if err := ensureDevxDir(); err != nil {
		return err
	}

	// Process services need connection env resolved up front; containers get it
	// through buildCompose.
	procProf := prof
	var before, after []string
	if prof.HasProcesses() {
		procProf = resolveProfile(manifest, prof)
		before, after, err = splitProcessPhases(procProf)
		if err != nil {
			return err
		}
	}

	var foreground *process.Spec
	if len(before) > 0 {
		if foreground, err = startProcesses(ctx, procProf, before, *attach); err != nil {
			return err
		}
	}

	var rt runtime.Runtime
	var composePath string
	enableTelemetry := *telemetry && !*noTelemetry && prof.HasContainers()
	if prof.HasContainers() {
		rt, err = selectRuntime(ctx)
		if err != nil {
			return err
		}

		lockfile, _ := lock.Load(lockFile)
		composePath = filepath.Join(devxDir, composeFile)
		if err := writeCompose(composePath, manifest, profName, prof, lockfile, enableTelemetry); err != nil {
			return err
		}

		if err := rt.Up(ctx, composePath, manifest.Project.Name, runtime.UpOptions{Build: *build, Pull: *pull}); err != nil {
			return err
		}

		if err := waitForHealth(prof); err != nil {
			return err
		}
	}

	if len(after) > 0 {
		fg, err := startProcesses(ctx, procProf, after, *attach)
		if err != nil {
			return err
		}
		if fg != nil {
			foreground = fg
		}
	}

	var bgCmds []*exec.Cmd
	if len(prof.Hooks.AfterUp) > 0 {
		if rt == nil {
			for _, h := range prof.Hooks.AfterUp {
				if h.Exec != "" {
					return fmt.Errorf("afterUp exec hook needs a container service, but profile %q has none; use a run hook", profName)
				}
			}
		}
		fmt.Println("Running afterUp hooks...")
		var err error
		bgCmds, err = runHooks(ctx, rt, composePath, manifest.Project.Name, prof.Hooks.AfterUp)
		if err != nil {
			return err
		}
	}

	rtName := ""
	if rt != nil {
		rtName = rt.Name()
	}
	if err := writeState(state{Profile: profName, Runtime: rtName, Telemetry: enableTelemetry}); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to write state: %v\n", err)
	}

	fmt.Println("Environment is up")
	if rt != nil {
		printLinks(ctx, rt, composePath, manifest.Project.Name)
	}
	printProcessLinks(prof)

	if foreground != nil {
		fmt.Printf("\nRunning %s in the foreground — Ctrl+C to stop.\n", foreground.Name)
		code, err := process.RunForeground(ctx, *foreground)
		if err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("%s exited with code %d", foreground.Name, code)
		}
		return nil
	}

	if len(bgCmds) > 0 {
		fmt.Println("\nBackground processes running — Ctrl+C to stop.")
		waitForBackground(bgCmds)
	}

	return nil
}

func runUpK8s(ctx context.Context, manifest *config.Manifest, profName string, prof *config.Profile) error {
	prof = resolveDepImages(prof)
	prof = resolveConnections(manifest, prof)
	output, err := k8s.Render(manifest, profName, prof, "")
	if err != nil {
		return err
	}

	if err := ensureDevxDir(); err != nil {
		return err
	}
	path := filepath.Join(devxDir, k8sFile)
	if err := os.WriteFile(path, []byte(output), 0600); err != nil {
		return err
	}

	if err := k8s.Apply(ctx, path); err != nil {
		return err
	}

	if err := writeState(state{Profile: profName, Runtime: "k8s", Telemetry: false}); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to write state: %v\n", err)
	}

	fmt.Println("Kubernetes resources applied")
	return nil
}
