package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dever-labs/devx/internal/localai"
	"github.com/dever-labs/devx/internal/task"
)

func runStatus(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "Emit status as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if !*jsonOut {
		printProjectHeader()
	}

	m, err := loadValidManifest()
	if err != nil {
		if !*jsonOut {
			fmt.Println()
		}
		return err
	}
	state, err := task.LoadState(stateFilePath())
	if err != nil {
		return err
	}

	type row struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Detail string `json:"detail,omitempty"`
	}
	var rows []row
	for name := range m.Tasks {
		entry, ok := state.Lookup(name)
		if !ok {
			rows = append(rows, row{Name: name, Status: "not running"})
			continue
		}
		status := "running"
		if entry.IsContainer {
			status, _ = containerStatus(ctx, entry.ContainerName)
		}
		detail := ""
		if entry.IsContainer {
			detail = entry.ContainerName
		}
		rows = append(rows, row{Name: name, Status: status, Detail: detail})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })

	if *jsonOut {
		data, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	maxLen := 0
	for _, r := range rows {
		if len(r.Name) > maxLen {
			maxLen = len(r.Name)
		}
	}
	for _, r := range rows {
		fmt.Printf("  %-*s  %-12s %s\n", maxLen, r.Name, r.Status, r.Detail)
	}
	return nil
}

func containerStatus(ctx context.Context, name string) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.State.Status}}", name).CombinedOutput()
	if err != nil {
		return "exited", nil
	}
	return strings.TrimSpace(string(out)), nil
}

func printProjectHeader() {
	projectName := ""
	if m, err := loadManifest(); err == nil {
		projectName = m.Project.Name
	}
	if projectName == "" {
		cwd, _ := os.Getwd()
		projectName = filepath.Base(cwd)
	}
	fmt.Printf("Project  %s\n", projectName)
	fmt.Printf("DevCon   %s\n", devcontainerState())
	fmt.Printf("AI       %s\n", aiStatusLine())
	fmt.Println()
}

func devcontainerState() string {
	if !fileExists(".devcontainer/devcontainer.json") && !fileExists("devcontainer.json") {
		return "not configured"
	}
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
