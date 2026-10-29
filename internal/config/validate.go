package config

import (
	"fmt"
	"sort"
	"strings"
)

type ValidationError struct {
	Issues []string
}

func (e *ValidationError) Error() string {
	return "manifest validation failed:\n- " + joinIssues(e.Issues)
}

func joinIssues(issues []string) string {
	if len(issues) == 0 {
		return ""
	}
	sort.Strings(issues)
	return strings.Join(issues, "\n- ")
}

var validScopes = map[string]bool{"": true, "workspace": true, "shared": true}
var validRestarts = map[string]bool{"": true, "never": true, "on-failure": true, "always": true}

// Validate checks the merged Manifest (after Load has resolved includes:)
// for structural problems: unknown deps, cycles are caught separately by
// the scheduler's graph build, field combinations that are never valid
// regardless of a task's inferred shape, and reserved-name collisions.
func Validate(m *Manifest) error {
	var issues []string
	if m.Version != 1 {
		issues = append(issues, "version must be 1")
	}
	if len(m.Tasks) == 0 {
		issues = append(issues, "at least one task is required")
	}
	if m.AI != nil {
		if m.AI.Provider == "" {
			issues = append(issues, "ai.provider is required when ai block is present")
		}
		if m.AI.Model == "" {
			issues = append(issues, "ai.model is required when ai block is present")
		}
	}

	for name, t := range m.Tasks {
		issues = append(issues, validateTask(m, name, t)...)
	}

	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

func validateTask(m *Manifest, name string, t Task) []string {
	var issues []string

	if t.Run == "" {
		issues = append(issues, fmt.Sprintf("task %q: run is required", name))
	}

	for _, dep := range t.Deps {
		if _, ok := m.Tasks[dep]; !ok {
			issues = append(issues, fmt.Sprintf("task %q: deps references unknown task %q", name, dep))
		}
	}

	if t.Interactive && t.Detached != nil && *t.Detached {
		issues = append(issues, fmt.Sprintf("task %q: interactive: true cannot be combined with detached: true", name))
	}

	if len(t.Generates) > 0 && t.Detached != nil && *t.Detached {
		issues = append(issues, fmt.Sprintf("task %q: generates: is invalid on a detached task (mtime \"up to date\" skip semantics only apply to one-shot recipes) — sources: alone is valid on detached tasks for --watch", name))
	}

	if !validRestarts[t.Restart] {
		issues = append(issues, fmt.Sprintf("task %q: restart must be never, on-failure, or always", name))
	}
	if t.Restart != "" && (t.Detached == nil || !*t.Detached) {
		issues = append(issues, fmt.Sprintf("task %q: restart is only meaningful on a detached task", name))
	}

	if !validScopes[t.Scope] {
		issues = append(issues, fmt.Sprintf("task %q: scope must be workspace or shared", name))
	}
	if t.Scope == "shared" {
		if strings.Contains(t.Run, "${workspace}") || strings.Contains(t.Run, "${port}") {
			issues = append(issues, fmt.Sprintf("task %q: scope: shared tasks cannot reference ${workspace}/${port} — a shared resource resolving differently per workspace defeats the point of sharing it", name))
		}
	}

	return issues
}

// ValidateTools checks that all tool declarations are well-formed.
func ValidateTools(m *Manifest) error {
	var issues []string
	for i, t := range m.Tools {
		if t.Name == "" {
			issues = append(issues, fmt.Sprintf("tools[%d]: name is required", i))
		}
		if t.Check == "" {
			label := t.Name
			if label == "" {
				label = fmt.Sprintf("[%d]", i)
			}
			issues = append(issues, fmt.Sprintf("tool '%s': check is required", label))
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// ValidateEnvFiles checks that all envFiles declarations are well-formed.
func ValidateEnvFiles(m *Manifest) error {
	var issues []string
	for i, f := range m.EnvFiles {
		label := f.Name
		if label == "" {
			label = fmt.Sprintf("envFiles[%d]", i)
		}
		if f.Template == "" {
			issues = append(issues, fmt.Sprintf("%s: template is required", label))
		}
		if f.Output == "" {
			issues = append(issues, fmt.Sprintf("%s: output is required", label))
		}
		for j, v := range f.Vars {
			if v.Name == "" {
				issues = append(issues, fmt.Sprintf("%s: vars[%d]: name is required", label, j))
			}
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// ValidateSetup checks that all setup step declarations are well-formed.
func ValidateSetup(m *Manifest) error {
	var issues []string
	validPlatforms := map[string]bool{"": true, "all": true, "windows": true, "linux": true, "macos": true, "darwin": true}
	for i, s := range m.Setup {
		if s.Name == "" {
			issues = append(issues, fmt.Sprintf("setup[%d]: name is required", i))
		}
		if s.Run == "" {
			issues = append(issues, fmt.Sprintf("setup step '%s': run is required", s.Name))
		}
		if !validPlatforms[s.Platform] {
			issues = append(issues, fmt.Sprintf("setup step '%s': platform must be all, windows, linux, or macos", s.Name))
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}
