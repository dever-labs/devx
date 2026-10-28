package main

import (
	"context"
	"flag"
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"github.com/dever-labs/devx/internal/config"
	"github.com/dever-labs/devx/internal/graph"
	"github.com/dever-labs/devx/internal/task"
)

func runDown(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("down", flag.ContinueOnError)
	force := fs.Bool("force", false, "Skip graceful shutdown, go straight to SIGKILL/docker kill")
	if err := fs.Parse(args); err != nil {
		return err
	}

	m, err := loadValidManifest()
	if err != nil {
		return err
	}
	state, err := task.LoadState(stateFilePath())
	if err != nil {
		return err
	}

	order, err := reverseDependencyOrder(m)
	if err != nil {
		return err
	}

	timeout := 10 * time.Second
	if *force {
		timeout = 0
	}

	anyStopped := false
	for _, name := range order {
		entry, ok := state.Lookup(name)
		if !ok {
			continue
		}
		fmt.Printf("Stopping %s...\n", name)
		if err := stopEntry(ctx, entry, timeout); err != nil {
			fmt.Printf("  warning: %v\n", err)
		}
		state.Remove(name)
		anyStopped = true
	}

	if !anyStopped {
		fmt.Println("Nothing is running.")
	} else {
		fmt.Println("Stack stopped.")
	}
	return state.Save(stateFilePath())
}

// reverseDependencyOrder returns every task name in reverse topological
// order (dependents before their dependencies), so devx down stops
// consumers before what they depend on.
func reverseDependencyOrder(m *config.Manifest) ([]string, error) {
	g, err := graph.Build(m.Tasks)
	if err != nil {
		return nil, err
	}
	order, err := graph.TopoSort(g)
	if err != nil {
		return nil, err
	}
	reversed := make([]string, len(order))
	for i, n := range order {
		reversed[len(order)-1-i] = n
	}
	return reversed, nil
}

func stopContainerByName(ctx context.Context, name string, timeout time.Duration) error {
	if name == "" {
		return fmt.Errorf("no containerName recorded — can't stop it by name")
	}
	runtime := "docker"
	if !lookPathQuiet("docker") && lookPathQuiet("podman") {
		runtime = "podman"
	}
	secs := int(timeout.Seconds())
	cmd := exec.CommandContext(ctx, runtime, "stop", "-t", fmt.Sprint(secs), name)
	return cmd.Run()
}

func stopEntry(ctx context.Context, entry task.StateEntry, timeout time.Duration) error {
	if entry.IsContainer {
		return stopContainerByName(ctx, entry.ContainerName, timeout)
	}
	if entry.PID <= 0 {
		// Started before PID tracking existed, or the driver couldn't
		// report one — nothing left to signal.
		return nil
	}
	return stopProcessGroup(entry.PID, timeout)
}

// stopProcessGroup signals the process group devx started the task in
// (see processDriver's Setpgid: true) — SIGTERM first, then SIGKILL if
// it hasn't exited by timeout. Each signal is a best-effort no-op if the
// group has already exited (ESRCH).
func stopProcessGroup(pid int, timeout time.Duration) error {
	pgid := -pid
	if err := syscall.Kill(pgid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return nil // process is gone
		}
		time.Sleep(200 * time.Millisecond)
	}
	if syscall.Kill(pid, 0) == nil {
		_ = syscall.Kill(pgid, syscall.SIGKILL)
	}
	return nil
}
