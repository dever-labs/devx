package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/dever-labs/devx/internal/config"
	"github.com/dever-labs/devx/internal/task"
)

// validateResult is the --json output shape for 'devx validate'.
type validateResult struct {
	Valid    bool     `json:"valid"`
	Issues   []string `json:"issues"`
	Warnings []string `json:"warnings"`
}

func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	file := fs.String("file", manifestFile, "Path to devx.yaml to validate")
	outputJSON := fs.Bool("json", false, "Emit result as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	manifest, err := config.Load(*file)
	if err != nil {
		if *outputJSON {
			printValidateJSON([]string{fmt.Sprintf("parse error: %v", err)}, nil)
		} else {
			fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
		}
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
	collect(config.ValidateEnvFiles(manifest))
	sort.Strings(issues)

	// extendedWarnings are informational only — they never fail
	// validation (see issue #58: a built-in-name collision or an
	// un-named one-shot container used to be fatal here, which is a
	// false positive since neither actually breaks anything).
	warnings := extendedWarnings(manifest)
	sort.Strings(warnings)

	if *outputJSON {
		printValidateJSON(issues, warnings)
		if len(issues) > 0 {
			return fmt.Errorf("validation failed")
		}
		return nil
	}

	if len(warnings) > 0 {
		fmt.Fprintf(os.Stderr, "devx.yaml has %d warning(s):\n", len(warnings))
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "  - %s\n", w)
		}
	}

	if len(issues) > 0 {
		fmt.Fprintf(os.Stderr, "devx.yaml has %d issue(s):\n", len(issues))
		for _, issue := range issues {
			fmt.Fprintf(os.Stderr, "  - %s\n", issue)
		}
		return fmt.Errorf("validation failed")
	}

	fmt.Println("devx.yaml is valid ✓")
	return nil
}

func printValidateJSON(issues, warnings []string) {
	if issues == nil {
		issues = []string{}
	}
	if warnings == nil {
		warnings = []string{}
	}
	data, err := json.MarshalIndent(validateResult{Valid: len(issues) == 0, Issues: issues, Warnings: warnings}, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error encoding JSON: %v\n", err)
		return
	}
	fmt.Println(string(data))
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
			warnings = append(warnings, fmt.Sprintf("task %q collides with a built-in command name — 'devx %s' will always run the built-in, not this task; use 'devx run %s' to run it explicitly", name, name, name))
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
		if et.IsContainer && et.ContainerName == "" && et.Detached {
			warnings = append(warnings, fmt.Sprintf("task %q looks like a container launch but no name could be parsed or overridden via containerName: — devx down/exec would have nothing to act on", name))
		}
	}
	for port, names := range ports {
		if len(names) > 1 {
			sort.Strings(names)
			unresolved := unconflictedPairs(m, names)
			if len(unresolved) > 1 {
				warnings = append(warnings, fmt.Sprintf("port %d is used by multiple tasks: %v", port, unresolved))
			}
		}
	}
	return warnings
}

// unconflictedPairs filters names down to the subset that isn't fully
// pairwise covered by conflicts: declarations — e.g. with three tasks on
// the same port where only two declare conflicts: on each other, the
// third is still reported. A single remaining name after filtering means
// every collision was intentional, so nothing is reported at all.
func unconflictedPairs(m *config.Manifest, names []string) []string {
	conflictsWith := func(a, b string) bool {
		for _, c := range m.Tasks[a].Conflicts {
			if c == b {
				return true
			}
		}
		for _, c := range m.Tasks[b].Conflicts {
			if c == a {
				return true
			}
		}
		return false
	}
	var out []string
	for _, n := range names {
		allDeclared := true
		for _, other := range names {
			if other == n {
				continue
			}
			if !conflictsWith(n, other) {
				allDeclared = false
				break
			}
		}
		if !allDeclared {
			out = append(out, n)
		}
	}
	return out
}

var builtinCommandNames = map[string]bool{
	"up": true, "down": true, "run": true, "status": true, "logs": true,
	"exec": true, "doctor": true, "validate": true, "tasks": true,
	"render": true, "lock": true, "providers": true, "export": true, "ai": true,
	"dev": true, "update": true, "completion": true, "ci": true,
	"env": true, "auth": true, "new": true, "init": true, "version": true, "help": true,
	"workspace": true, "ps": true,
}
