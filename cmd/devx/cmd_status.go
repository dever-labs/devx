package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dever-labs/devx/internal/localai"
	"github.com/dever-labs/devx/internal/runtime"
	"github.com/dever-labs/devx/internal/ui"
)

func runStatus(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	outputJSON := fs.Bool("json", false, "Emit status as JSON")
	_ = fs.Parse(args)

	// ── Project header ────────────────────────────────────────────────────────
	if !*outputJSON {
		printProjectHeader()
	}

	manifest, profName, prof, err := loadProfile("")
	if err != nil {
		if !*outputJSON {
			fmt.Println()
		}
		return err
	}

	if profileRuntime(prof) == "k8s" {
		return errors.New("status for k8s runtime is not supported yet")
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

	statuses, err := rt.Status(ctx, composePath, manifest.Project.Name)
	if err != nil {
		return err
	}

	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Name < statuses[j].Name
	})

	if *outputJSON {
		return printStatusJSON(statuses)
	}

	headers := []string{"Service", "State", "Health", "Ports"}
	rows := make([][]string, 0, len(statuses))
	for _, st := range statuses {
		rows = append(rows, []string{st.Name, st.State, st.Health, st.Ports})
	}
	ui.PrintTable(os.Stdout, headers, rows)
	return nil
}

// printProjectHeader shows project name, devcontainer status, and AI backend.
func printProjectHeader() {
	// Project name from manifest (best-effort).
	projectName := ""
	if m, err := loadManifestOnly(); err == nil {
		projectName = m.Project.Name
	}
	if projectName == "" {
		cwd, _ := os.Getwd()
		projectName = filepath.Base(cwd)
	}
	fmt.Printf("Project  %s\n", projectName)

	// Devcontainer state.
	dcState := devcontainerState()
	fmt.Printf("DevCon   %s\n", dcState)

	// AI state.
	aiState := aiStatusLine()
	fmt.Printf("AI       %s\n", aiState)

	fmt.Println()
}

func devcontainerState() string {
	// Check for devcontainer.json.
	if !fileExists(".devcontainer/devcontainer.json") && !fileExists("devcontainer.json") {
		return "not configured"
	}

	// Use docker ps to check for a running devcontainer.
	cwd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	out, err := exec.Command("docker", "ps", "--filter",
		"label=devcontainer.local_folder="+cwd,
		"--format", "{{.Names}}").Output()
	if err != nil {
		return "configured (docker unavailable)"
	}
	names := strings.TrimSpace(string(out))
	if names == "" {
		return "configured, not running — use 'devx dev up'"
	}
	return "running (" + strings.ReplaceAll(names, "\n", ", ") + ")"
}

func aiStatusLine() string {
	cfg, err := localai.LoadSavedAIConfig()
	if err != nil || cfg == nil {
		return "not configured — run 'devx ai'"
	}

	line := fmt.Sprintf("provider=%s tool=%s", cfg.Provider, cfg.Tool)
	if cfg.Model != "" {
		line += " model=" + cfg.Model
	}

	// For local/remote backends, probe the backend.
	if cfg.Provider == "local" || cfg.Provider == "remote" {
		status, _ := localai.DetectWithEndpoint(cfg.Backend, cfg.Endpoint)
		if status != nil {
			line += fmt.Sprintf("  ✓ %s @ %s", status.Backend, status.URL)
		} else {
			line += "  ✗ backend not running"
		}
	}
	return line
}

func printStatusJSON(statuses []runtime.ServiceStatus) error {
	type jsonStatus struct {
		Name   string `json:"name"`
		State  string `json:"state"`
		Health string `json:"health"`
		Ports  string `json:"ports"`
	}
	out := make([]jsonStatus, len(statuses))
	for i, s := range statuses {
		out[i] = jsonStatus{Name: s.Name, State: s.State, Health: s.Health, Ports: s.Ports}
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
