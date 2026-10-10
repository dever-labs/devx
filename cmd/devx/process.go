package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/dever-labs/devx/internal/config"
	"github.com/dever-labs/devx/internal/graph"
	"github.com/dever-labs/devx/internal/process"
	"github.com/dever-labs/devx/internal/ready"
)

func supervisor() *process.Supervisor { return process.New(devxDir) }

// hostPort returns the host-side port of a compose-style port mapping
// ("3000", "8080:80", "127.0.0.1:8080:80").
func hostPort(mapping string) string {
	parts := strings.Split(mapping, ":")
	switch len(parts) {
	case 1:
		return parts[0]
	case 2:
		return parts[0]
	default:
		return parts[1]
	}
}

// hostVals rewrites connection template values so a host process reaches a
// containerised dep through its published port on localhost.
func hostVals(vals map[string]string, depPorts []string) map[string]string {
	out := make(map[string]string, len(vals))
	for k, v := range vals {
		out[k] = v
	}
	out["host"] = "localhost"
	if len(depPorts) > 0 {
		out["port"] = hostPort(depPorts[0])
	}
	return out
}

func processSpec(name string, svc config.Service) process.Spec {
	return process.Spec{
		Name:    name,
		Command: svc.Command,
		Run:     svc.Run,
		Workdir: svc.Workdir,
		Env:     svc.Env,
	}
}

// probeFor builds the readiness probe for a service. Process services that
// publish a port fall back to a TCP check so "up" really means reachable.
func probeFor(name string, svc config.Service) ready.Probe {
	p := ready.Probe{Name: name}
	if r := svc.Ready; r != nil {
		p.HTTP, p.TCP = r.HTTP, r.TCP
		p.Interval = parseDuration(r.Interval)
		p.Timeout = parseDuration(r.Timeout)
		return p
	}
	if svc.Health != nil && svc.Health.HttpGet != "" {
		p.HTTP = svc.Health.HttpGet
		p.Interval = parseDuration(svc.Health.Interval)
		return p
	}
	if svc.IsProcess() && len(svc.Ports) > 0 {
		p.TCP = "localhost:" + hostPort(svc.Ports[0])
	}
	return p
}

func parseDuration(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	return d
}

// processNames returns process service names in dependency order.
func processNames(prof *config.Profile) ([]string, error) {
	g, err := graph.Build(prof)
	if err != nil {
		return nil, err
	}
	order, err := graph.TopoSort(g)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, n := range order {
		if svc, ok := prof.Services[n]; ok && svc.IsProcess() {
			names = append(names, n)
		}
	}
	return names, nil
}

// needsContainers reports whether name (transitively) depends on a container
// service or dep, meaning it can only start once compose is up.
func needsContainers(prof *config.Profile, name string, seen map[string]bool) bool {
	if seen[name] {
		return false
	}
	seen[name] = true
	svc, ok := prof.Services[name]
	if !ok {
		return true
	}
	for _, d := range svc.DependsOn {
		dep, isSvc := prof.Services[d]
		if !isSvc || !dep.IsProcess() || needsContainers(prof, d, seen) {
			return true
		}
	}
	return false
}

// splitProcessPhases separates process services that must start before compose
// (containers may depend on them) from those that wait for containers.
func splitProcessPhases(prof *config.Profile) (before, after []string, err error) {
	names, err := processNames(prof)
	if err != nil {
		return nil, nil, err
	}
	for _, n := range names {
		if needsContainers(prof, n, map[string]bool{}) {
			after = append(after, n)
		} else {
			before = append(before, n)
		}
	}
	return before, after, nil
}

// startProcesses starts the named process services in order, waiting for each
// to become ready before starting the next.
func startProcesses(ctx context.Context, prof *config.Profile, names []string, attach string) (*process.Spec, error) {
	sup := supervisor()
	var foreground *process.Spec
	for _, name := range names {
		svc := prof.Services[name]
		spec := processSpec(name, svc)
		if name == attach {
			s := spec
			foreground = &s
			continue
		}
		rec, started, err := sup.Start(spec)
		if err != nil {
			return nil, err
		}
		if started {
			fmt.Printf("Started %s (pid %d) — logs: %s\n", name, rec.PID, rec.LogFile)
		} else {
			fmt.Printf("%s already running (pid %d)\n", name, rec.PID)
		}
		probe := probeFor(name, svc)
		probe.Alive = func() bool { return sup.Alive(name) }
		if err := ready.Wait(ctx, []ready.Probe{probe}); err != nil {
			for _, l := range sup.Tail(name, 15) {
				fmt.Fprintf(os.Stderr, "  %s | %s\n", name, l)
			}
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	return foreground, nil
}

// waitForHealth waits for the readiness of every container service that
// declares a probe (ready: or health.httpGet).
func waitForHealth(profile *config.Profile) error {
	if profile == nil {
		return nil
	}
	var probes []ready.Probe
	for _, name := range sortedServiceNames(profile) {
		svc := profile.Services[name]
		if svc.IsProcess() {
			continue
		}
		if p := probeFor(name, svc); !p.Empty() {
			probes = append(probes, p)
		}
	}
	return ready.Wait(context.Background(), probes)
}

func sortedServiceNames(prof *config.Profile) []string {
	names := make([]string, 0, len(prof.Services))
	for n := range prof.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// stopProcesses stops all supervised processes (reverse start order).
func stopProcesses() error {
	sup := supervisor()
	if len(sup.List()) == 0 {
		return nil
	}
	fmt.Println("Stopping processes...")
	return sup.StopAll(10 * time.Second)
}

// printProcessLinks prints URLs for process services that publish a port.
func printProcessLinks(prof *config.Profile) {
	type link struct{ label, url string }
	var links []link
	for _, name := range sortedServiceNames(prof) {
		svc := prof.Services[name]
		if !svc.IsProcess() {
			continue
		}
		for _, p := range svc.Ports {
			links = append(links, link{serviceLabel(name), "http://localhost:" + hostPort(p)})
		}
	}
	if len(links) == 0 {
		return
	}
	fmt.Println("\nProcess services:")
	for _, l := range links {
		fmt.Printf("  %s  %s\n", l.label, l.url)
	}
}

func resolveProfile(manifest *config.Manifest, prof *config.Profile) *config.Profile {
	return resolveConnections(manifest, resolveDepImages(prof))
}
