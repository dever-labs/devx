// Package detect infers required tool versions from files already present in
// a repository (package.json, .nvmrc, global.json, go.mod, ...), so that
// devx.yaml does not have to duplicate them.
package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Tool is a tool requirement discovered in the repository.
type Tool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"`
}

// Dir scans dir and returns detected tools. Earlier, more specific sources
// win over later ones for the same tool name.
func Dir(dir string) []Tool {
	var out []Tool
	seen := map[string]bool{}
	add := func(name, version, source string) {
		version = strings.TrimSpace(version)
		if name == "" || version == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, Tool{Name: name, Version: version, Source: source})
	}
	read := func(name string) (string, bool) {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return "", false
		}
		return string(b), true
	}

	// .tool-versions (asdf/mise) — explicit per-repo pins.
	if s, ok := read(".tool-versions"); ok {
		for _, line := range strings.Split(s, "\n") {
			f := strings.Fields(strings.SplitN(line, "#", 2)[0])
			if len(f) < 2 {
				continue
			}
			name := f[0]
			switch name {
			case "nodejs":
				name = "node"
			case "golang":
				name = "go"
			}
			add(name, f[1], ".tool-versions")
		}
	}

	for _, f := range []string{".nvmrc", ".node-version"} {
		if s, ok := read(f); ok {
			add("node", strings.TrimPrefix(firstLine(s), "v"), f)
		}
	}

	if s, ok := read("package.json"); ok {
		var pkg struct {
			Engines        map[string]string `json:"engines"`
			PackageManager string            `json:"packageManager"`
		}
		if json.Unmarshal([]byte(s), &pkg) == nil {
			add("node", cleanRange(pkg.Engines["node"]), "package.json#engines.node")
			if pm, v, ok := strings.Cut(pkg.PackageManager, "@"); ok {
				v, _, _ = strings.Cut(v, "+")
				add(pm, v, "package.json#packageManager")
			}
			for _, pm := range []string{"yarn", "pnpm", "npm"} {
				add(pm, cleanRange(pkg.Engines[pm]), "package.json#engines."+pm)
			}
		}
	}

	if s, ok := read(".python-version"); ok {
		add("python", firstLine(s), ".python-version")
	}

	if s, ok := read("global.json"); ok {
		var g struct {
			SDK struct {
				Version string `json:"version"`
			} `json:"sdk"`
		}
		if json.Unmarshal([]byte(s), &g) == nil {
			add("dotnet", g.SDK.Version, "global.json")
		}
	}

	if s, ok := read("go.mod"); ok {
		for _, line := range strings.Split(s, "\n") {
			if f := strings.Fields(line); len(f) == 2 && f[0] == "go" {
				add("go", f[1], "go.mod")
				break
			}
		}
	}
	return out
}

// Find returns the detected tool with the given name.
func Find(tools []Tool, name string) (Tool, bool) {
	for _, t := range tools {
		if strings.EqualFold(t.Name, name) {
			return t, true
		}
	}
	return Tool{}, false
}

var verRe = regexp.MustCompile(`\d+(\.\d+){0,2}`)

// cleanRange reduces a semver range like ">=18.12 <21" to its first version.
func cleanRange(r string) string {
	return verRe.FindString(r)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	l, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(l)
}

// DefaultCheck returns the conventional version command for a known tool.
func DefaultCheck(name string) string {
	switch strings.ToLower(name) {
	case "node", "yarn", "pnpm", "npm", "python", "go", "dotnet":
		cmd := strings.ToLower(name)
		switch cmd {
		case "go":
			return "go version"
		case "python":
			return "python3 --version"
		}
		return cmd + " --version"
	}
	return ""
}

// VersionMatches reports whether the installed version output satisfies the
// wanted version, comparing only the components given in want (so "20" matches
// "v20.11.1").
func VersionMatches(want, output string) bool {
	want = strings.TrimPrefix(strings.TrimSpace(want), "v")
	got := verRe.FindString(output)
	if want == "" || got == "" {
		return true
	}
	w := strings.Split(cleanRange(want), ".")
	g := strings.Split(got, ".")
	for i := range w {
		if i >= len(g) || w[i] != g[i] {
			return false
		}
	}
	return true
}
