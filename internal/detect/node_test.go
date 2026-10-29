package detect

import (
	"testing"
)

func TestNodeVersion_FromNvmrc(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".nvmrc", "v20.11.0\n")

	v, ok := NodeVersion(dir)
	if !ok {
		t.Fatal("expected NodeVersion to detect .nvmrc")
	}
	if v != "20.11.0" {
		t.Errorf("NodeVersion() = %q, want %q", v, "20.11.0")
	}
}

func TestNodeVersion_FromNodeVersionFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".node-version", "22.11.0")

	v, ok := NodeVersion(dir)
	if !ok {
		t.Fatal("expected NodeVersion to detect .node-version")
	}
	if v != "22.11.0" {
		t.Errorf("NodeVersion() = %q, want %q", v, "22.11.0")
	}
}

func TestNodeVersion_NvmrcWinsOverPackageJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".nvmrc", "18.20.0")
	writeFile(t, dir, "package.json", `{"engines": {"node": "^20.0.0"}}`)

	v, ok := NodeVersion(dir)
	if !ok {
		t.Fatal("expected NodeVersion to detect a version")
	}
	if v != "18.20.0" {
		t.Errorf("NodeVersion() = %q, want %q (.nvmrc should win)", v, "18.20.0")
	}
}

func TestNodeVersion_FromPackageJSONEngines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name": "app", "engines": {"node": ">=20.11.0"}}`)

	v, ok := NodeVersion(dir)
	if !ok {
		t.Fatal("expected NodeVersion to detect package.json engines.node")
	}
	if v != "20.11.0" {
		t.Errorf("NodeVersion() = %q, want %q", v, "20.11.0")
	}
}

func TestNodeVersion_NoSourcesReturnsNotOK(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name": "app"}`)

	if _, ok := NodeVersion(dir); ok {
		t.Error("expected NodeVersion to report not-found when no version pin exists")
	}
}

func TestNodeVersion_NoFilesAtAllReturnsNotOK(t *testing.T) {
	dir := t.TempDir()

	if _, ok := NodeVersion(dir); ok {
		t.Error("expected NodeVersion to report not-found for an empty dir")
	}
}

func TestNodeVersion_MalformedPackageJSONReturnsNotOK(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{not valid json`)

	if _, ok := NodeVersion(dir); ok {
		t.Error("expected NodeVersion to report not-found for malformed package.json")
	}
}

func TestNodeInstallCommand(t *testing.T) {
	cases := []struct {
		manager, version, want string
	}{
		{"volta", "20.11.0", "volta install node@20.11.0"},
		{"fnm", "20.11.0", "fnm install 20.11.0 && fnm use 20.11.0"},
		{"nvm", "20.11.0", "nvm install 20.11.0"},
		{"unknown", "20.11.0", ""},
	}
	for _, c := range cases {
		got := NodeInstallCommand(c.manager, c.version)
		if got != c.want {
			t.Errorf("NodeInstallCommand(%q, %q) = %q, want %q", c.manager, c.version, got, c.want)
		}
	}
}

func TestCorepackInstallCommand_NotOnPATH(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if cmd, ok := CorepackInstallCommand("yarn@4.1.0"); ok {
		t.Errorf("expected CorepackInstallCommand to report not-found with empty PATH, got %q", cmd)
	}
}

func TestCleanVersion(t *testing.T) {
	cases := map[string]string{
		"20.11.0":   "20.11.0",
		"v20.11.0":  "20.11.0",
		"^20.11.0":  "20.11.0",
		"~20.11.0":  "20.11.0",
		">=20.11.0": "20.11.0",
		" 20.11.0 ": "20.11.0",
	}
	for in, want := range cases {
		if got := cleanVersion(in); got != want {
			t.Errorf("cleanVersion(%q) = %q, want %q", in, got, want)
		}
	}
}
