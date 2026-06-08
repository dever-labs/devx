package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	goruntime "runtime"
	"sort"
	"strings"

	"github.com/dever-labs/devx/internal/config"
	"github.com/dever-labs/devx/internal/k8s"
	"github.com/dever-labs/devx/internal/localai"
	"github.com/dever-labs/devx/internal/runtime"
	"github.com/dever-labs/devx/internal/runtime/docker"
	"github.com/dever-labs/devx/internal/runtime/podman"
	"github.com/dever-labs/devx/internal/setup"
)

type Options struct {
	Manifest *config.Manifest
	Fix      bool
}

type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type Report struct {
	Checks []Check
}

func (r Report) HasFailures() bool {
	for _, c := range r.Checks {
		if c.Status == "FAIL" {
			return true
		}
	}
	return false
}

func Run(ctx context.Context, opts Options) Report {
	checks := []Check{}

	checks = append(checks, Check{
		Name:   "CLI",
		Status: "PASS",
		Detail: fmt.Sprintf("devx (dev) on %s/%s", goruntime.GOOS, goruntime.GOARCH),
	})

	runtimeChecks := detectAllRuntimes(ctx)
	for _, info := range runtimeChecks {
		status := "FAIL"
		if info.Available {
			status = "PASS"
		}
		detail := info.Details
		if detail == "" {
			detail = info.Name
		}
		checks = append(checks, Check{
			Name:   fmt.Sprintf("Runtime: %s", info.Name),
			Status: status,
			Detail: detail,
		})

		if info.Available {
			checks = append(checks, detectCompose(ctx, info.Name))
		}
	}

	if opts.Manifest != nil {
		checks = append(checks, checkPortConflicts(opts.Manifest))
		if opts.Manifest.Registry.Prefix != "" {
			checks = append(checks, checkRegistry(opts.Manifest.Registry.Prefix))
		}
		if requiresK8s(opts.Manifest) {
			checks = append(checks, checkKubectl())
		}
		// Check tools declared in the manifest.
		toolChecks := checkTools(ctx, opts.Manifest, opts.Fix)
		checks = append(checks, toolChecks...)
	}

	// Devcontainer CLI + AI backend checks (independent of manifest).
	checks = append(checks, checkDevcontainerCLI())
	checks = append(checks, checkAI()...)

	sort.SliceStable(checks, func(i, j int) bool {
		return checks[i].Name < checks[j].Name
	})

	return Report{Checks: checks}
}

func PrintReport(out *os.File, report Report, asJSON bool) {
	if asJSON {
		data, err := json.MarshalIndent(report.Checks, "", "  ")
		if err != nil {
			fmt.Fprintf(out, "error encoding JSON: %v\n", err)
			return
		}
		fmt.Fprintln(out, string(data))
		return
	}
	for _, check := range report.Checks {
		icon := doctorIcon(check.Status)
		if check.Detail != "" {
			fmt.Fprintf(out, "%s  %-30s  %s\n", icon, check.Name, check.Detail)
		} else {
			fmt.Fprintf(out, "%s  %s\n", icon, check.Name)
		}
	}
}

func doctorIcon(status string) string {
	switch status {
	case "PASS":
		return "✓"
	case "WARN":
		return "!"
	default:
		return "✗"
	}
}

// checkTools checks each tool declared in the manifest. If fix is true,
// missing tools are installed via devx/setup before reporting.
func checkTools(ctx context.Context, m *config.Manifest, fix bool) []Check {
	if len(m.Tools) == 0 {
		return nil
	}
	opts := setup.Options{Fix: fix}
	results := setup.RunTools(ctx, m.Tools, opts)
	checks := make([]Check, 0, len(results))
	for _, r := range results {
		label := fmt.Sprintf("Tool: %s", r.Name)
		switch r.Status {
		case "ok", "installed":
			checks = append(checks, Check{Name: label, Status: "PASS", Detail: r.Detail})
		case "missing":
			hint := r.Detail
			if hint == "" {
				hint = "not found — run 'devx setup --fix' to install"
			}
			checks = append(checks, Check{Name: label, Status: "FAIL", Detail: hint})
		case "failed":
			checks = append(checks, Check{Name: label, Status: "FAIL", Detail: r.Detail})
		default:
			checks = append(checks, Check{Name: label, Status: "WARN", Detail: r.Detail})
		}
	}
	return checks
}

func detectAllRuntimes(ctx context.Context) []runtime.RuntimeInfo {
	var infos []runtime.RuntimeInfo
	for _, rt := range []runtime.Runtime{docker.New(), podman.New()} {
		ok, err := rt.Detect(ctx)
		info := runtime.RuntimeInfo{Name: rt.Name(), Available: ok}
		if err != nil {
			info.Details = err.Error()
		}
		infos = append(infos, info)
	}
	return infos
}

func detectCompose(ctx context.Context, runtimeName string) Check {
	binary := runtimeName
	cmd := exec.CommandContext(ctx, binary, "compose", "version")
	if err := cmd.Run(); err != nil {
		return Check{
			Name:   fmt.Sprintf("Compose: %s", runtimeName),
			Status: "WARN",
			Detail: "compose not available",
		}
	}

	return Check{
		Name:   fmt.Sprintf("Compose: %s", runtimeName),
		Status: "PASS",
		Detail: "compose available",
	}
}

func checkPortConflicts(manifest *config.Manifest) Check {
	ports := map[string][]string{}
	for _, prof := range manifest.Profiles {
		for name, svc := range prof.Services {
			for _, port := range svc.Ports {
				host := strings.Split(port, ":")[0]
				ports[host] = append(ports[host], name)
			}
		}
		for name, dep := range prof.Deps {
			for _, port := range dep.Ports {
				host := strings.Split(port, ":")[0]
				ports[host] = append(ports[host], name)
			}
		}
	}

	var conflicts []string
	for host, services := range ports {
		if len(services) > 1 {
			conflicts = append(conflicts, fmt.Sprintf("port %s used by %s", host, strings.Join(services, ", ")))
		}
	}

	if len(conflicts) == 0 {
		return Check{Name: "Ports", Status: "PASS", Detail: "no duplicate ports"}
	}

	return Check{Name: "Ports", Status: "WARN", Detail: strings.Join(conflicts, "; ")}
}

func checkRegistry(prefix string) Check {
	if os.Getenv("DEVX_OFFLINE") == "1" {
		return Check{Name: "Registry", Status: "WARN", Detail: "offline mode enabled"}
	}

	domain := strings.Split(prefix, "/")[0]
	_, err := net.LookupHost(domain)
	if err != nil {
		return Check{Name: "Registry", Status: "WARN", Detail: "registry not reachable"}
	}

	return Check{Name: "Registry", Status: "PASS", Detail: "registry reachable"}
}

func requiresK8s(manifest *config.Manifest) bool {
	for _, profile := range manifest.Profiles {
		if profile.Runtime == "k8s" {
			return true
		}
	}
	return false
}

func checkKubectl() Check {
	if err := k8s.DetectKubectl(); err != nil {
		return Check{Name: "kubectl", Status: "WARN", Detail: "kubectl not found"}
	}

	return Check{Name: "kubectl", Status: "PASS", Detail: "kubectl available"}
}

// checkDevcontainerCLI verifies the @devcontainers/cli npm package is installed.
func checkDevcontainerCLI() Check {
	cmd := exec.Command("devcontainer", "--version")
	if err := cmd.Run(); err != nil {
		return Check{
			Name:   "devcontainer CLI",
			Status: "WARN",
			Detail: "not found — install with: npm install -g @devcontainers/cli",
		}
	}
	return Check{Name: "devcontainer CLI", Status: "PASS", Detail: "devcontainer CLI available"}
}

// checkAI reads .devx/ai.yaml and probes the configured AI backend.
func checkAI() []Check {
	cfg, err := localai.LoadSavedAIConfig()
	if err != nil {
		return []Check{{Name: "AI config", Status: "WARN", Detail: "could not read .devx/ai.yaml: " + err.Error()}}
	}
	if cfg == nil {
		return []Check{{Name: "AI config", Status: "WARN", Detail: "not configured — run 'devx ai' to set up"}}
	}

	checks := []Check{{
		Name:   "AI config",
		Status: "PASS",
		Detail: fmt.Sprintf("provider=%s tool=%s model=%s", cfg.Provider, cfg.Tool, cfg.Model),
	}}

	// Check the AI tool binary exists.
	toolBin := map[string]string{
		"aider":      "aider",
		"claude-code": "claude",
		"codex":      "codex",
		"gh-copilot": "gh",
	}
	if bin, ok := toolBin[cfg.Tool]; ok {
		if _, lookErr := exec.LookPath(bin); lookErr != nil {
			checks = append(checks, Check{
				Name:   "AI tool: " + cfg.Tool,
				Status: "WARN",
				Detail: bin + " not found in PATH",
			})
		} else {
			checks = append(checks, Check{Name: "AI tool: " + cfg.Tool, Status: "PASS"})
		}
	}

	// For local/remote providers, probe the backend.
	if cfg.Provider == "local" || cfg.Provider == "remote" {
		status, probeErr := localai.DetectWithEndpoint(cfg.Backend, cfg.Endpoint)
		if probeErr != nil {
			checks = append(checks, Check{Name: "AI backend", Status: "WARN", Detail: probeErr.Error()})
		} else if status == nil {
			hint := "run 'devx ai setup' to start"
			if cfg.Provider == "remote" {
				hint = "check that " + cfg.Endpoint + " is reachable"
			}
			checks = append(checks, Check{Name: "AI backend", Status: "WARN", Detail: "not running — " + hint})
		} else {
			detail := fmt.Sprintf("%s at %s", status.Backend, status.URL)
			if status.Model != "" {
				detail += " model=" + status.Model
			}
			checks = append(checks, Check{Name: "AI backend", Status: "PASS", Detail: detail})
		}
	}

	return checks
}

