// Package detect infers project conventions (package manager, workspace
// layout, pinned tool versions) from files already present in a repo, so
// devx.yaml doesn't have to duplicate information that can drift from the
// real source of truth (package.json, lockfiles, version-manager config
// files, etc).
package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// JSWorkspace describes a detected Node/JS package-manager setup.
type JSWorkspace struct {
	// Manager is the inferred package manager: "npm", "yarn", or "pnpm".
	Manager string
	// IsWorkspace is true when the root package.json declares a
	// "workspaces" field, or a pnpm-workspace.yaml file is present.
	IsWorkspace bool
	// Packages holds the raw workspace globs (package.json "workspaces",
	// or pnpm-workspace.yaml's "packages:"), informational only.
	Packages []string
	// ManagerVersion is the full Corepack "packageManager" spec (e.g.
	// "yarn@4.1.0"), set only when Manager was inferred from that field
	// rather than a lockfile. Empty otherwise.
	ManagerVersion string
}

// packageJSON is the subset of package.json fields detection needs.
type packageJSON struct {
	PackageManager string          `json:"packageManager"`
	Workspaces     json.RawMessage `json:"workspaces"`
}

// workspacesObject is package.json's alternate (non-array) "workspaces"
// shape: {"packages": [...]}.
type workspacesObject struct {
	Packages []string `json:"packages"`
}

// JS inspects dir for a package.json and reports the detected package
// manager and workspace layout. Returns (nil, false) if dir has no
// package.json at all (not a JS/TS project, or not yet scaffolded).
func JS(dir string) (*JSWorkspace, bool) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil, false
	}

	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, false
	}

	ws := &JSWorkspace{Manager: detectManager(dir, pkg.PackageManager)}
	if pkg.PackageManager != "" && strings.HasPrefix(pkg.PackageManager, ws.Manager+"@") {
		ws.ManagerVersion = pkg.PackageManager
	}

	switch {
	case len(pkg.Workspaces) > 0:
		ws.IsWorkspace = true
		ws.Packages = parseWorkspacesField(pkg.Workspaces)
	case fileExists(filepath.Join(dir, "pnpm-workspace.yaml")):
		ws.IsWorkspace = true
		ws.Manager = "pnpm"
	}

	return ws, true
}

// parseWorkspacesField handles both package.json "workspaces" shapes:
// a plain array of globs, or {"packages": [...]}.
func parseWorkspacesField(raw json.RawMessage) []string {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	var obj workspacesObject
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.Packages
	}
	return nil
}

// detectManager infers npm/yarn/pnpm, preferring the explicit Corepack
// "packageManager" field (e.g. "yarn@4.1.0"), then falling back to
// whichever lockfile is present, then defaulting to npm.
func detectManager(dir, packageManagerField string) string {
	if packageManagerField != "" {
		for i, c := range packageManagerField {
			if c == '@' {
				return packageManagerField[:i]
			}
		}
		return packageManagerField
	}
	switch {
	case fileExists(filepath.Join(dir, "pnpm-lock.yaml")):
		return "pnpm"
	case fileExists(filepath.Join(dir, "yarn.lock")):
		return "yarn"
	default:
		return "npm"
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// InstallCommand returns the single root-level install command for ws's
// detected manager — the same command whether or not this is a
// workspace, since every supported manager installs every workspace
// package's dependencies from a single root invocation.
func (ws *JSWorkspace) InstallCommand() string {
	switch ws.Manager {
	case "yarn":
		return "yarn install"
	case "pnpm":
		return "pnpm install"
	default:
		return "npm install"
	}
}
