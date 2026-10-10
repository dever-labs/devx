package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dever-labs/devx/internal/config"
	"github.com/dever-labs/devx/internal/detect"
)

// ResolveTools fills gaps in tool declarations from files in dir. Explicit
// values in devx.yaml always win; when an explicit version differs from the
// detected one, Conflict is set so doctor can warn.
func ResolveTools(dir string, tools []config.Tool) []config.Tool {
	found := detect.Dir(dir)
	out := make([]config.Tool, len(tools))
	for i, t := range tools {
		if d, ok := detect.Find(found, t.Name); ok {
			if t.Version == "" {
				t.Version, t.Source = d.Version, d.Source
			} else if !detect.VersionMatches(t.Version, d.Version) && !detect.VersionMatches(d.Version, t.Version) {
				t.Source = "devx.yaml"
				t.Conflict = fmt.Sprintf("devx.yaml requires %s but %s says %s", t.Version, d.Source, d.Version)
			} else {
				t.Source = "devx.yaml"
			}
		} else if t.Version != "" {
			t.Source = "devx.yaml"
		}
		if t.Check == "" {
			t.Check = detect.DefaultCheck(t.Name)
		}
		out[i] = t
	}
	return out
}

// versionManagerInstall returns an install command using a version manager
// found on this machine, or "" when none applies.
func versionManagerInstall(t config.Tool) string {
	if t.Version == "" || t.Installer == "system" {
		return ""
	}
	has := func(bin string) bool { _, err := exec.LookPath(bin); return err == nil }
	switch strings.ToLower(t.Name) {
	case "node":
		switch {
		case has("volta"):
			return "volta install node@" + t.Version
		case has("fnm"):
			return fmt.Sprintf("fnm install %s && fnm use %s", t.Version, t.Version)
		case nvmScript() != "":
			return fmt.Sprintf(". %q && nvm install %s && nvm use %s", nvmScript(), t.Version, t.Version)
		}
	case "yarn", "pnpm":
		if has("corepack") {
			return fmt.Sprintf("corepack enable && corepack prepare %s@%s --activate", t.Name, t.Version)
		}
	}
	return ""
}

func nvmScript() string {
	dir := os.Getenv("NVM_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".nvm")
	}
	p := filepath.Join(dir, "nvm.sh")
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// ResolveSteps expands `workspace: auto` steps into a single root install
// command. When no steps are declared and dir is a JS workspace, one
// install step is synthesised so `devx setup` works with zero configuration.
func ResolveSteps(dir string, steps []config.SetupStep) []config.SetupStep {
	ws, found := detect.JSWorkspace(dir)
	if len(steps) == 0 {
		if !found {
			return nil
		}
		return []config.SetupStep{{Name: "install (" + ws.Manager + " workspace)", Run: ws.Install}}
	}
	out := make([]config.SetupStep, len(steps))
	for i, s := range steps {
		if s.Workspace == "auto" {
			if found {
				s.Run = ws.Install
			} else {
				s.Run = "echo no JS workspace detected"
			}
		}
		out[i] = s
	}
	return out
}
