package config

import (
	"fmt"
	"github.com/dever-labs/devx/internal/detect"
	"sort"
	"strings"
	"time"
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
	if m.AI != nil {
		if m.AI.Provider == "" {
			issues = append(issues, "ai.provider is required when ai block is present")
		}
		if m.AI.Model == "" {
			issues = append(issues, "ai.model is required when ai block is present")
		}
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
		issues = append(issues, validateServiceKind(profile, prof, name, svc)...)
	}

	for name, dep := range prof.Deps {
		if dep.Kind == "" && dep.Image == "" {
			issues = append(issues, fmt.Sprintf("dep '%s' must define image (or set kind to use a provider's default image)", name))
		}
		if dep.Kind != "" && dep.Version == "" {
			issues = append(issues, fmt.Sprintf("dep '%s' has kind '%s' but is missing version — version is required when kind is set", name, dep.Kind))
		}
		if dep.Source != "" && !strings.Contains(dep.Source, "/") {
			issues = append(issues, fmt.Sprintf("dep '%s' source must be in org/name format (e.g. devx-labs/postgres)", name))
		}
		for i, c := range dep.Connect {
			if c.Service == "" {
				issues = append(issues, fmt.Sprintf("dep '%s' connect[%d] must specify a service", name, i))
			} else if _, ok := prof.Services[c.Service]; !ok {
				issues = append(issues, fmt.Sprintf("dep '%s' connect[%d] references service '%s' which does not exist", name, i, c.Service))
			}
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
		if h.Background && hasExec {
			issues = append(issues, fmt.Sprintf("hook[%d] background is only supported for run hooks", i))
		}
		if h.Name != "" && hasExec {
			issues = append(issues, fmt.Sprintf("hook[%d] name is only supported for run hooks", i))
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

// ValidateTools checks that all tool declarations are well-formed.
func ValidateTools(m *Manifest) error {
	var issues []string
	for i, t := range m.Tools {
		if t.Name == "" {
			issues = append(issues, fmt.Sprintf("tools[%d]: name is required", i))
		}
		if t.Installer != "" && t.Installer != "auto" && t.Installer != "system" {
			issues = append(issues, fmt.Sprintf("tool '%s': installer must be auto or system", t.Name))
		}
		if t.Check == "" && detect.DefaultCheck(t.Name) == "" {
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

// ValidateSetup checks that all setup step declarations are well-formed.
func ValidateSetup(m *Manifest) error {
	var issues []string
	validPlatforms := map[string]bool{"": true, "all": true, "windows": true, "linux": true, "macos": true, "darwin": true}
	for i, s := range m.Setup {
		if s.Name == "" {
			issues = append(issues, fmt.Sprintf("setup[%d]: name is required", i))
		}
		if s.Run == "" && s.Workspace == "" {
			issues = append(issues, fmt.Sprintf("setup step '%s': run is required", s.Name))
		}
		if s.Workspace != "" && s.Workspace != "auto" {
			issues = append(issues, fmt.Sprintf("setup step '%s': workspace must be auto", s.Name))
		}
		if s.Workspace != "" && s.Run != "" {
			issues = append(issues, fmt.Sprintf("setup step '%s': cannot set both run and workspace", s.Name))
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

func validateServiceKind(profileName string, prof Profile, name string, svc Service) []string {
	var issues []string
	switch svc.Kind {
	case "", KindContainer:
		if svc.Image == "" && svc.Build == nil {
			issues = append(issues, fmt.Sprintf("service '%s' must define image or build", name))
		}
		if svc.Run != "" {
			issues = append(issues, fmt.Sprintf("service '%s' run is only supported for kind: process", name))
		}
	case KindProcess:
		if svc.Image != "" || svc.Build != nil {
			issues = append(issues, fmt.Sprintf("service '%s' kind process cannot define image or build", name))
		}
		if len(svc.Command) == 0 && svc.Run == "" {
			issues = append(issues, fmt.Sprintf("service '%s' kind process must define command or run", name))
		}
		if len(svc.Command) > 0 && svc.Run != "" {
			issues = append(issues, fmt.Sprintf("service '%s' cannot set both command and run", name))
		}
		if len(svc.Mount) > 0 {
			issues = append(issues, fmt.Sprintf("service '%s' mount is not supported for kind process", name))
		}
		if prof.Runtime == "k8s" {
			issues = append(issues, fmt.Sprintf("profile '%s' uses runtime k8s, which does not support process service '%s'", profileName, name))
		}
	default:
		issues = append(issues, fmt.Sprintf("service '%s' kind must be container or process", name))
	}

	for _, dep := range svc.DependsOn {
		if !existsServiceOrDep(prof, dep) {
			issues = append(issues, fmt.Sprintf("service '%s' dependsOn '%s' which does not exist", name, dep))
		}
	}

	if r := svc.Ready; r != nil {
		if r.HTTP == "" && r.TCP == "" {
			issues = append(issues, fmt.Sprintf("service '%s' ready must set http or tcp", name))
		}
		for field, v := range map[string]string{"interval": r.Interval, "timeout": r.Timeout} {
			if v == "" {
				continue
			}
			if d, err := time.ParseDuration(v); err != nil || d <= 0 {
				issues = append(issues, fmt.Sprintf("service '%s' ready.%s must be a positive duration like 2s or 1m", name, field))
			}
		}
	}
	if svc.Debug != nil {
		if svc.Debug.Port <= 0 || svc.Debug.Port > 65535 {
			issues = append(issues, fmt.Sprintf("service '%s' debug.port must be 1-65535", name))
		}
		switch svc.Debug.Type {
		case "", "node", "python", "go", "java":
		default:
			issues = append(issues, fmt.Sprintf("service '%s' debug.type must be node, python, go, or java", name))
		}
	}
	return issues
}

// ValidateEnvFiles checks envFiles declarations.
func ValidateEnvFiles(m *Manifest) error {
	var issues []string
	for i, f := range m.EnvFiles {
		if f.File == "" {
			issues = append(issues, fmt.Sprintf("envFiles[%d]: file is required", i))
		}
		if len(f.Values) == 0 {
			issues = append(issues, fmt.Sprintf("envFiles[%d]: values is required", i))
		}
		for k, s := range f.Values {
			switch s.From {
			case "env", "prompt":
			case "command":
				if s.Run == "" {
					issues = append(issues, fmt.Sprintf("envFiles[%d].values.%s: run is required for from: command", i, k))
				}
			default:
				issues = append(issues, fmt.Sprintf("envFiles[%d].values.%s: from must be env, command, or prompt", i, k))
			}
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}
