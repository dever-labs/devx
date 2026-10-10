package detect

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDir(t *testing.T) {
	d := t.TempDir()
	write(t, d, ".nvmrc", "v20.11.1\n")
	write(t, d, "package.json", `{"engines":{"node":">=18 <21"},"packageManager":"yarn@4.1.0+sha256.abc"}`)
	write(t, d, "global.json", `{"sdk":{"version":"8.0.100"}}`)
	write(t, d, "go.mod", "module x\n\ngo 1.21\n")
	write(t, d, ".tool-versions", "python 3.12.1 # c\n")
	got := Dir(d)
	want := map[string]string{"node": "20.11.1", "yarn": "4.1.0", "dotnet": "8.0.100", "go": "1.21", "python": "3.12.1"}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for _, g := range got {
		if want[g.Name] != g.Version {
			t.Errorf("%s: got %s want %s", g.Name, g.Version, want[g.Name])
		}
	}
	if n, _ := Find(got, "node"); n.Source != ".nvmrc" {
		t.Errorf("source = %s", n.Source)
	}
}

func TestVersionMatches(t *testing.T) {
	cases := []struct {
		want, out string
		ok        bool
	}{
		{"20", "v20.11.1", true},
		{"20.11", "v20.12.0", false},
		{"1.21", "go version go1.21.5 linux/amd64", true},
		{"", "x", true},
	}
	for _, c := range cases {
		if VersionMatches(c.want, c.out) != c.ok {
			t.Errorf("%q vs %q", c.want, c.out)
		}
	}
}

func TestJSWorkspace(t *testing.T) {
	d := t.TempDir()
	if _, ok := JSWorkspace(d); ok {
		t.Fatal("empty dir is not a workspace")
	}
	write(t, d, "package.json", `{"workspaces":["packages/*"],"packageManager":"yarn@4.1.0"}`)
	write(t, d, ".yarnrc.yml", "")
	w, ok := JSWorkspace(d)
	if !ok || w.Manager != "yarn" || w.Install != "yarn install --immutable" {
		t.Fatalf("got %+v %v", w, ok)
	}
	d2 := t.TempDir()
	write(t, d2, "pnpm-workspace.yaml", "packages: ['a']")
	if w, _ := JSWorkspace(d2); w.Install != "pnpm install" {
		t.Fatalf("got %+v", w)
	}
}
