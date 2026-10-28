package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/dever-labs/devx/internal/config"
	"github.com/dever-labs/devx/internal/task"
)

func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	file := fs.String("file", manifestFile, "Path to devx.yaml to validate")
	if err := fs.Parse(args); err != nil {
		return err
	}

	manifest, err := config.Load(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
		return fmt.Errorf("validation failed")
	}

	var issues []string
	collect := func(err error) {
		if err == nil {
			return
		}
		if ve, ok := err.(*config.ValidationError); ok {
			issues = append(issues, ve.Issues...)
			return
		}
		issues = append(issues, err.Error())
	}

	collect(config.Validate(manifest))
	collect(config.ValidateTools(manifest))
	collect(config.ValidateSetup(manifest))
	issues = append(issues, extendedWarnings(manifest)...)

	if len(issues) > 0 {
		sort.Strings(issues)
		fmt.Fprintf(os.Stderr, "devx.yaml has %d issue(s):\n", len(issues))
		for _, issue := range issues {
			fmt.Fprintf(os.Stderr, "  - %s\n", issue)
		}
		return fmt.Errorf("validation failed")
	}

	fmt.Println("devx.yaml is valid ✓")
	return nil
}

// extendedWarnings covers the design doc's "Extended checks (all
// warnings, not errors)" list: built-in command name collisions, a
// detached task with no inferrable readiness, a container-shaped run:
// with no parseable/overridden containerName:, and port collisions
// (surfaced via internal/doctor's own check too, repeated here so
// `validate` alone catches it without running `devx doctor`).
func extendedWarnings(m *config.Manifest) []string {
	var warnings []string
	ports := map[int][]string{}

	for name, t := range m.Tasks {
		if builtinCommandNames[name] {
			warnings = append(warnings, fmt.Sprintf("task %q collides with a built-in command name — 'devx %s' will always run the built-in, not this task", name, name))
		}

		et, err := task.Resolve(t, task.NewResolver(nil, nil, t.ScopeVars))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("task %q: %v", name, err))
			continue
		}
		if et.Port > 0 {
			ports[et.Port] = append(ports[et.Port], name)
		}
		if et.Detached && et.ResolvedReady == nil {
			warnings = append(warnings, fmt.Sprintf("task %q looks long-running but no port/readiness could be inferred — add ready: or port: explicitly", name))
		}
		if et.IsContainer && et.ContainerName == "" {
			warnings = append(warnings, fmt.Sprintf("task %q looks like a container launch but no name could be parsed or overridden via containerName: — devx down/exec would have nothing to act on", name))
		}
	}
	for port, names := range ports {
		if len(names) > 1 {
			sort.Strings(names)
			warnings = append(warnings, fmt.Sprintf("port %d is used by multiple tasks: %v", port, names))
		}
	}
	return warnings
}

var builtinCommandNames = map[string]bool{
	"up": true, "down": true, "run": true, "status": true, "logs": true,
	"exec": true, "doctor": true, "setup": true, "validate": true, "tasks": true,
	"render": true, "lock": true, "providers": true, "export": true, "ai": true,
	"dev": true, "clone": true, "update": true, "completion": true, "ci": true,
	"env": true, "auth": true, "new": true, "init": true, "version": true, "help": true,
	"workspace": true, "ps": true,
}
