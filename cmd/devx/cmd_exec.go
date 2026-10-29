package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/dever-labs/devx/internal/task"
)

func runExec(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("exec requires a task name")
	}
	sep := -1
	for i, a := range args {
		if a == "--" {
			sep = i
			break
		}
	}
	if sep == -1 || sep == len(args)-1 {
		return errors.New("exec requires a command after --")
	}

	name := args[0]
	cmdArgs := args[sep+1:]

	m, err := loadValidManifest()
	if err != nil {
		return err
	}
	if _, ok := m.Tasks[name]; !ok {
		return fmt.Errorf("unknown task %q", name)
	}

	state, err := task.LoadState(stateFilePath())
	if err != nil {
		return err
	}
	entry, ok := state.Lookup(name)
	if !ok {
		return fmt.Errorf("task %q isn't currently running", name)
	}
	if !entry.IsContainer {
		return fmt.Errorf("task %q is a plain process, not a container — exec isn't supported for process-shaped tasks in this phase", name)
	}

	runtime := "docker"
	if !lookPathQuiet("docker") && lookPathQuiet("podman") {
		runtime = "podman"
	}
	full := append([]string{"exec", "-it", entry.ContainerName}, cmdArgs...)
	cmd := exec.CommandContext(ctx, runtime, full...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
