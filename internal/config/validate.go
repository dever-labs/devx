package config

import (
	"fmt"
	"sort"
	"strings"
)

var supportedDeps = map[string]bool{
	"postgres": true,
	"redis":    true,
}

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

func Validate(m *Manifest) error {
	var issues []string
	if m.Version != 1 {
		issues = append(issues, "version must be 1")
	}
	if m.Project.Name == "" {
		issues = append(issues, "project.name is required")
	}
	if m.Project.DefaultProfile == "" {
		issues = append(issues, "project.defaultProfile is required")
	}
	if len(m.Profiles) == 0 {
		issues = append(issues, "profiles are required")
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	if _, ok := m.Profiles[m.Project.DefaultProfile]; !ok {
		return &ValidationError{Issues: []string{"project.defaultProfile does not exist"}}
	}
	return nil
}

func ValidateProfile(m *Manifest, profile string) error {
	prof, ok := m.Profiles[profile]
	if !ok {
		return &ValidationError{Issues: []string{"profile does not exist"}}
	}

	var issues []string
	if prof.Runtime != "" && prof.Runtime != "compose" && prof.Runtime != "k8s" {
		issues = append(issues, fmt.Sprintf("profile '%s' runtime must be compose or k8s", profile))
	}
	for name, svc := range prof.Services {
		if svc.Image == "" && svc.Build == nil {
			issues = append(issues, fmt.Sprintf("service '%s' must define image or build", name))
		}
		for _, dep := range svc.DependsOn {
			if !existsServiceOrDep(prof, dep) {
				issues = append(issues, fmt.Sprintf("service '%s' dependsOn '%s' which does not exist", name, dep))
			}
		}
	}

	for name, dep := range prof.Deps {
		if dep.Kind == "" {
			issues = append(issues, fmt.Sprintf("dep '%s' must define kind", name))
		} else if !supportedDeps[dep.Kind] {
			issues = append(issues, fmt.Sprintf("dep '%s' kind '%s' is not supported", name, dep.Kind))
		}
	}

	allHooks := append(prof.Hooks.AfterUp, prof.Hooks.BeforeDown...)
	for i, h := range allHooks {
		hasExec := h.Exec != ""
		hasRun := h.Run != ""
		if !hasExec && !hasRun {
			issues = append(issues, fmt.Sprintf("hook[%d] must set either exec or run", i))
		}
		if hasExec && hasRun {
			issues = append(issues, fmt.Sprintf("hook[%d] cannot set both exec and run", i))
		}
		if hasExec && h.Service == "" {
			issues = append(issues, fmt.Sprintf("hook[%d] exec requires service to be set", i))
		}
		if hasRun && h.Service != "" {
			issues = append(issues, fmt.Sprintf("hook[%d] run does not use service", i))
		}
	}

	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

func existsServiceOrDep(prof Profile, name string) bool {
	if _, ok := prof.Services[name]; ok {
		return true
	}
	if _, ok := prof.Deps[name]; ok {
		return true
	}
	return false
}
