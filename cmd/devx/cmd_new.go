package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dever-labs/devx/internal/config"
	"gopkg.in/yaml.v3"
)

func runNew(_ context.Context, args []string) error {
	if len(args) == 0 {
		return runNewInteractive()
	}
	switch args[0] {
	case "service":
		return runNewService()
	case "dep":
		return runNewDep()
	default:
		return fmt.Errorf("unknown new subcommand %q — use: service | dep", args[0])
	}
}

func runNewInteractive() error {
	reader := bufio.NewReader(os.Stdin)
	idx, err := promptChoice(reader, "What do you want to add?",
		[]string{
			"service  — a containerised application built from source",
			"dep      — a third-party dependency (database, cache, broker…)",
		}, 0)
	if err != nil {
		return err
	}
	fmt.Println()
	if idx == 0 {
		return runNewService()
	}
	return runNewDep()
}

// ── Add service ───────────────────────────────────────────────────────────────

func runNewService() error {
	reader := bufio.NewReader(os.Stdin)

	name, err := promptString(reader, "Service name", "api")
	if err != nil {
		return err
	}

	useImage, err := promptYN(reader, "Use a pre-built image (not build from source)?", false)
	if err != nil {
		return err
	}

	svc := config.Service{}
	if useImage {
		image, err := promptString(reader, "Image", "nginx:alpine")
		if err != nil {
			return err
		}
		svc.Image = image
	} else {
		ctx, err := promptString(reader, "Build context", "./"+name)
		if err != nil {
			return err
		}
		df, err := promptString(reader, "Dockerfile path", "Dockerfile")
		if err != nil {
			return err
		}
		svc.Build = &config.Build{Context: ctx, Dockerfile: df}
	}

	portsStr, err := promptString(reader, "Port mapping (e.g. 8080:8080, leave empty to skip)", "")
	if err != nil {
		return err
	}
	if portsStr != "" {
		svc.Ports = []string{portsStr}
	}

	flagsFS := flag.NewFlagSet("new service", flag.ContinueOnError)
	_ = flagsFS.Parse(nil)

	return applyToManifest(func(m *config.Manifest) error {
		profile := m.Project.DefaultProfile
		if profile == "" {
			profile = "local"
		}
		p, ok := m.Profiles[profile]
		if !ok {
			p = config.Profile{Services: map[string]config.Service{}}
		}
		if p.Services == nil {
			p.Services = map[string]config.Service{}
		}
		p.Services[name] = svc
		if m.Profiles == nil {
			m.Profiles = map[string]config.Profile{}
		}
		m.Profiles[profile] = p
		return nil
	}, fmt.Sprintf("service %q", name))
}

// ── Add dep ───────────────────────────────────────────────────────────────────

func runNewDep() error {
	reader := bufio.NewReader(os.Stdin)

	name, err := promptString(reader, "Dependency name", "db")
	if err != nil {
		return err
	}

	kindIdx, err := promptChoice(reader, "Type?",
		[]string{"postgres", "redis", "mysql", "rabbitmq", "mongodb", "other (enter manually)"},
		0)
	if err != nil {
		return err
	}
	kinds := []string{"postgres", "redis", "mysql", "rabbitmq", "mongodb", ""}
	kind := kinds[kindIdx]
	if kind == "" {
		kind, err = promptString(reader, "Kind", "")
		if err != nil {
			return err
		}
	}

	version, err := promptString(reader, "Version", defaultVersion(kind))
	if err != nil {
		return err
	}

	dep := config.Dep{Kind: kind, Version: version}
	switch kind {
	case "postgres":
		dep.Ports = []string{"5432:5432"}
		dep.Env = map[string]string{"POSTGRES_PASSWORD": "postgres"}
		dep.Volume = "db-data:/var/lib/postgresql/data"
	case "redis":
		dep.Ports = []string{"6379:6379"}
	case "mysql":
		dep.Ports = []string{"3306:3306"}
		dep.Env = map[string]string{"MYSQL_ROOT_PASSWORD": "mysql"}
		dep.Volume = "db-data:/var/lib/mysql"
	case "rabbitmq":
		dep.Ports = []string{"5672:5672", "15672:15672"}
	case "mongodb":
		dep.Ports = []string{"27017:27017"}
		dep.Volume = "mongo-data:/data/db"
	}

	return applyToManifest(func(m *config.Manifest) error {
		profile := m.Project.DefaultProfile
		if profile == "" {
			profile = "local"
		}
		p, ok := m.Profiles[profile]
		if !ok {
			p = config.Profile{}
		}
		if p.Deps == nil {
			p.Deps = map[string]config.Dep{}
		}
		p.Deps[name] = dep
		if m.Profiles == nil {
			m.Profiles = map[string]config.Profile{}
		}
		m.Profiles[profile] = p
		return nil
	}, fmt.Sprintf("dep %q (%s:%s)", name, kind, version))
}

func defaultVersion(kind string) string {
	switch kind {
	case "postgres":
		return "16"
	case "redis":
		return "7"
	case "mysql":
		return "8"
	case "mongodb":
		return "7"
	default:
		return ""
	}
}

// applyToManifest reads devx.yaml, applies fn, and writes it back.
func applyToManifest(fn func(*config.Manifest) error, desc string) error {
	if !fileExists(manifestFile) {
		return fmt.Errorf("devx.yaml not found — run  devx init  first")
	}

	raw, err := os.ReadFile(manifestFile)
	if err != nil {
		return err
	}

	// Round-trip through yaml.Node to preserve comments and field order.
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("parsing devx.yaml: %w", err)
	}

	// Parse into struct for mutation.
	m, err := config.Parse(raw)
	if err != nil {
		return fmt.Errorf("parsing devx.yaml: %w", err)
	}

	if err := fn(m); err != nil {
		return err
	}

	out, err := yaml.Marshal(m)
	if err != nil {
		return err
	}

	// Preserve $schema header if present.
	header := ""
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "# yaml-language-server") || strings.HasPrefix(line, "#") {
			header += line + "\n"
		} else {
			break
		}
	}

	if err := os.WriteFile(manifestFile, []byte(header+string(out)), 0644); err != nil {
		return err
	}

	fmt.Printf("✓  added %s to devx.yaml\n", desc)
	return nil
}
