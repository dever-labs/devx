package doctor

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	goruntime "runtime"
	"sort"
	"strings"

	"github.com/dever-labs/devx/internal/config"
	"github.com/dever-labs/devx/internal/k8s"
	"github.com/dever-labs/devx/internal/runtime"
	"github.com/dever-labs/devx/internal/runtime/docker"
	"github.com/dever-labs/devx/internal/runtime/podman"
)

type Options struct {
	Manifest *config.Manifest
	Fix      bool
}

type Check struct {
	Name   string
	Status string
	Detail string
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
	}

	sort.SliceStable(checks, func(i, j int) bool {
		return checks[i].Name < checks[j].Name
	})

	return Report{Checks: checks}
}

func PrintReport(out *os.File, report Report) {
	for _, check := range report.Checks {
		fmt.Fprintf(out, "%s\t%s\t%s\n", check.Status, check.Name, check.Detail)
	}
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
