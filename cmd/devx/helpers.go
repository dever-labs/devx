package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dever-labs/devx/internal/config"
	devxruntime "github.com/dever-labs/devx/internal/runtime"
	"github.com/dever-labs/devx/internal/runtime/docker"
	"github.com/dever-labs/devx/internal/runtime/podman"
	"github.com/dever-labs/devx/internal/task"
)

// loadManifest loads and merges devx.yaml (resolving includes:) without
// validating it — use this when a command wants to report parse errors
// itself (e.g. validate) rather than failing generically.
func loadManifest() (*config.Manifest, error) {
	return config.Load(manifestFile)
}

// loadValidManifest loads, merges, and validates devx.yaml — the common
// case for every command that actually runs tasks.
func loadValidManifest() (*config.Manifest, error) {
	m, err := loadManifest()
	if err != nil {
		return nil, err
	}
	if err := config.Validate(m); err != nil {
		return nil, err
	}
	return m, nil
}

func ensureDevxDir() error {
	return os.MkdirAll(devxDir, 0755)
}

func ensureGitignore() error {
	path := ".gitignore"
	entry := ".devx/"

	if !fileExists(path) {
		return os.WriteFile(path, []byte(entry+"\n"), 0644)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if strings.Contains(string(data), entry) {
		return nil
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString("\n" + entry + "\n")
	return err
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// stateFilePath returns .devx/tasks.json — the task runtime registry
// (see internal/task.StateStore) — not to be confused with the old
// compose-era state.json, which no longer exists.
func stateFilePath() string {
	return filepath.Join(devxDir, "tasks.json")
}

func logDirPath() string {
	return filepath.Join(devxDir, "logs")
}

// newScheduler builds a task.Scheduler for the current project with the
// given CLI var overrides (KEY=VALUE pairs from the command line).
func newScheduler(m *config.Manifest, cliVars map[string]string, opts task.Options) (*task.Scheduler, error) {
	opts.CLIVars = cliVars
	opts.LogDir = logDirPath()
	opts.StateFile = stateFilePath()
	return task.NewScheduler(m, opts)
}

// parseCLIVars splits "KEY=VALUE" trailing arguments into a map, and
// returns the remaining non-assignment arguments (task names) separately.
func parseCLIVars(args []string) (names []string, vars map[string]string) {
	vars = map[string]string{}
	for _, a := range args {
		if i := strings.Index(a, "="); i > 0 && !strings.ContainsAny(a[:i], "/.") {
			vars[a[:i]] = a[i+1:]
			continue
		}
		names = append(names, a)
	}
	return names, vars
}

// selectContainerRuntime detects docker or podman — used only by
// commands that still need to shell out directly (devx dev, doctor),
// not by the task engine itself (container-shaped tasks address docker/
// podman directly via the parsed/overridden runtime in run:).
func selectContainerRuntime(ctx context.Context) (devxruntime.Runtime, error) {
	d := docker.New()
	if ok, _ := d.Detect(ctx); ok {
		return d, nil
	}
	p := podman.New()
	if ok, _ := p.Detect(ctx); ok {
		return p, nil
	}
	return nil, devxruntime.ErrNoRuntime
}

// lookPathQuiet is a small wrapper kept for parity with the pre-rewrite
// helpers.go naming used elsewhere (e.g. cmd_ci.go's stack detection).
func lookPathQuiet(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}
