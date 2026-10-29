package detect

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestJS_NoPackageJSONReturnsNotOK(t *testing.T) {
	dir := t.TempDir()
	ws, ok := JS(dir)
	if ok || ws != nil {
		t.Errorf("JS(%q) = %+v, %v, want nil, false", dir, ws, ok)
	}
}

func TestJS_PlainPackageDefaultsToNPM(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name": "app"}`)

	ws, ok := JS(dir)
	if !ok {
		t.Fatal("expected JS to detect package.json")
	}
	if ws.Manager != "npm" {
		t.Errorf("Manager = %q, want npm", ws.Manager)
	}
	if ws.IsWorkspace {
		t.Error("expected IsWorkspace = false for a plain package.json")
	}
	if ws.InstallCommand() != "npm install" {
		t.Errorf("InstallCommand() = %q, want %q", ws.InstallCommand(), "npm install")
	}
}

func TestJS_LockfileDetectsYarn(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name": "app"}`)
	writeFile(t, dir, "yarn.lock", "")

	ws, ok := JS(dir)
	if !ok {
		t.Fatal("expected JS to detect package.json")
	}
	if ws.Manager != "yarn" {
		t.Errorf("Manager = %q, want yarn", ws.Manager)
	}
	if ws.InstallCommand() != "yarn install" {
		t.Errorf("InstallCommand() = %q, want %q", ws.InstallCommand(), "yarn install")
	}
}

func TestJS_LockfileDetectsPnpm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name": "app"}`)
	writeFile(t, dir, "pnpm-lock.yaml", "")

	ws, ok := JS(dir)
	if !ok {
		t.Fatal("expected JS to detect package.json")
	}
	if ws.Manager != "pnpm" {
		t.Errorf("Manager = %q, want pnpm", ws.Manager)
	}
}

func TestJS_PackageManagerFieldWinsOverLockfile(t *testing.T) {
	dir := t.TempDir()
	// Deliberately conflicting: a yarn.lock present, but packageManager
	// (Corepack's source of truth) says pnpm — the explicit field must win.
	writeFile(t, dir, "package.json", `{"name": "app", "packageManager": "pnpm@9.1.0"}`)
	writeFile(t, dir, "yarn.lock", "")

	ws, ok := JS(dir)
	if !ok {
		t.Fatal("expected JS to detect package.json")
	}
	if ws.Manager != "pnpm" {
		t.Errorf("Manager = %q, want pnpm (from packageManager field)", ws.Manager)
	}
}

func TestJS_ArrayWorkspacesFieldDetectsWorkspace(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name": "app", "workspaces": ["packages/*"]}`)

	ws, ok := JS(dir)
	if !ok {
		t.Fatal("expected JS to detect package.json")
	}
	if !ws.IsWorkspace {
		t.Error("expected IsWorkspace = true for an array workspaces field")
	}
	if len(ws.Packages) != 1 || ws.Packages[0] != "packages/*" {
		t.Errorf("Packages = %v, want [packages/*]", ws.Packages)
	}
}

func TestJS_ObjectWorkspacesFieldDetectsWorkspace(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name": "app", "workspaces": {"packages": ["apps/*", "libs/*"]}}`)

	ws, ok := JS(dir)
	if !ok {
		t.Fatal("expected JS to detect package.json")
	}
	if !ws.IsWorkspace {
		t.Error("expected IsWorkspace = true for an object workspaces field")
	}
	if len(ws.Packages) != 2 {
		t.Errorf("Packages = %v, want 2 entries", ws.Packages)
	}
}

func TestJS_PnpmWorkspaceYAMLDetectsWorkspaceAndManager(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name": "app"}`)
	writeFile(t, dir, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")

	ws, ok := JS(dir)
	if !ok {
		t.Fatal("expected JS to detect package.json")
	}
	if !ws.IsWorkspace {
		t.Error("expected IsWorkspace = true when pnpm-workspace.yaml is present")
	}
	if ws.Manager != "pnpm" {
		t.Errorf("Manager = %q, want pnpm", ws.Manager)
	}
}

func TestJS_MalformedPackageJSONReturnsNotOK(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", "{not valid json")

	ws, ok := JS(dir)
	if ok || ws != nil {
		t.Errorf("JS(%q) = %+v, %v, want nil, false", dir, ws, ok)
	}
}
