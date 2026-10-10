package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Workspace describes a JS monorepo root and the single command that installs
// every package in it.
type Workspace struct {
	Manager string `json:"manager"`
	Install string `json:"install"`
}

// JSWorkspace detects npm/yarn/pnpm workspaces rooted at dir. Installing once
// at the root is correct for all of them; per-package installs are not.
func JSWorkspace(dir string) (Workspace, bool) {
	exists := func(n string) bool { _, err := os.Stat(filepath.Join(dir, n)); return err == nil }

	hasWorkspaces := exists("pnpm-workspace.yaml") || exists("lerna.json")
	var pkg struct {
		Workspaces     json.RawMessage `json:"workspaces"`
		PackageManager string          `json:"packageManager"`
	}
	if b, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
		if json.Unmarshal(b, &pkg) == nil && len(pkg.Workspaces) > 0 && string(pkg.Workspaces) != "null" {
			hasWorkspaces = true
		}
	}
	if !hasWorkspaces {
		return Workspace{}, false
	}

	pm := ""
	if name, _ := splitPM(pkg.PackageManager); name != "" {
		pm = name
	}
	switch {
	case pm != "":
	case exists("pnpm-lock.yaml") || exists("pnpm-workspace.yaml"):
		pm = "pnpm"
	case exists("yarn.lock") || exists(".yarnrc.yml"):
		pm = "yarn"
	default:
		pm = "npm"
	}

	switch pm {
	case "pnpm":
		return Workspace{pm, "pnpm install"}, true
	case "yarn":
		if exists(".yarnrc.yml") {
			return Workspace{pm, "yarn install --immutable"}, true
		}
		return Workspace{pm, "yarn install"}, true
	default:
		if exists("package-lock.json") {
			return Workspace{"npm", "npm ci"}, true
		}
		return Workspace{"npm", "npm install"}, true
	}
}

func splitPM(s string) (string, string) {
	for i := 0; i < len(s); i++ {
		if s[i] == '@' {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}
