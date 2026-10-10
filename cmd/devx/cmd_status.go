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

	"github.com/dever-labs/devx/internal/config"
	"github.com/dever-labs/devx/internal/localai"
	"github.com/dever-labs/devx/internal/ui"
)

// statusRow is one line of `devx status`, covering containers and processes.
type statusRow struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	State     string `json:"state"`
	Health    string `json:"health"`
	Ports     string `json:"ports"`
	PID       int    `json:"pid,omitempty"`
	DebugPort int    `json:"debugPort,omitempty"`
	LogFile   string `json:"logFile,omitempty"`
}

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

	var rows []statusRow
	if prof.HasContainers() {
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
		for _, st := range statuses {
			row := statusRow{Name: st.Name, Kind: config.KindContainer, State: st.State, Health: st.Health, Ports: st.Ports}
			if svc, ok := prof.Services[st.Name]; ok && svc.Debug != nil {
				row.DebugPort = svc.Debug.Port
			}
			rows = append(rows, row)
		}
	}
	rows = append(rows, processRows(ctx, prof)...)

	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })

	if *outputJSON {
		return printStatusJSON(rows)
	}

	headers := []string{"Service", "Kind", "State", "Health", "Ports"}
	table := make([][]string, 0, len(rows))
	for _, r := range rows {
		table = append(table, []string{r.Name, r.Kind, r.State, r.Health, r.Ports})
	}
	ui.PrintTable(os.Stdout, headers, table)
	return nil
}

// processRows reports the state of every process service in the profile.
func processRows(ctx context.Context, prof *config.Profile) []statusRow {
	sup := supervisor()
	var rows []statusRow
	for _, name := range sortedServiceNames(prof) {
		svc := prof.Services[name]
		if !svc.IsProcess() {
			continue
		}
		row := statusRow{Name: name, Kind: config.KindProcess, State: "stopped", Ports: strings.Join(svc.Ports, ",")}
		if svc.Debug != nil {
			row.DebugPort = svc.Debug.Port
		}
		if rec, ok, running := sup.Get(name); ok {
			row.PID, row.LogFile = rec.PID, rec.LogFile
			row.State = "exited"
			if running {
				row.State = "running"
				if p := probeFor(name, svc); !p.Empty() {
					row.Health = "unhealthy"
					if p.Check(ctx) {
						row.Health = "healthy"
					}
				}
			}
		}
		rows = append(rows, row)
	}
	return rows
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

func printStatusJSON(rows []statusRow) error {
	if rows == nil {
		rows = []statusRow{}
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
