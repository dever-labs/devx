package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func writeInitTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestWriteInitManifest_WritesParseableYAML(t *testing.T) {
	dir := chdirTemp(t)

	for _, stack := range []string{"dotnet", "node", "go", "python", "other"} {
		t.Run(stack, func(t *testing.T) {
			if err := writeInitManifest("my-app", stack, false); err != nil {
				t.Fatalf("writeInitManifest(%q) failed: %v", stack, err)
			}
			data, err := os.ReadFile(filepath.Join(dir, manifestFile))
			if err != nil {
				t.Fatalf("reading manifest: %v", err)
			}
			var out map[string]interface{}
			if err := yaml.Unmarshal(data, &out); err != nil {
				t.Fatalf("writeInitManifest(%q) produced invalid YAML: %v\n%s", stack, err, data)
			}
			if out["version"] != 1 {
				t.Errorf("expected version: 1, got %v", out["version"])
			}
			if _, ok := out["build"]; !ok && stack != "python" {
				t.Errorf("expected a 'build' task for stack %q, got %v", stack, out)
			}
			_ = os.Remove(filepath.Join(dir, manifestFile))
		})
	}
}

func TestWriteInitManifest_WithAI(t *testing.T) {
	dir := chdirTemp(t)
	if err := writeInitManifest("my-app", "go", true); err != nil {
		t.Fatalf("writeInitManifest failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]interface{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("produced invalid YAML: %v\n%s", err, data)
	}
	if _, ok := out["ai"]; !ok {
		t.Errorf("expected an 'ai:' block when withAI=true, got %v", out)
	}
}

func TestWriteInitManifest_NodeWithYarnWorkspaceAddsRootSetupStep(t *testing.T) {
	dir := chdirTemp(t)
	writeInitTestFile(t, dir, "package.json", `{"name": "monorepo", "workspaces": ["packages/*"], "packageManager": "yarn@4.1.0"}`)

	if err := writeInitManifest("my-app", "node", false); err != nil {
		t.Fatalf("writeInitManifest failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Setup []struct {
			Name    string `yaml:"name"`
			Run     string `yaml:"run"`
			RunOnce bool   `yaml:"runOnce"`
		} `yaml:"setup"`
		Build struct {
			Run string `yaml:"run"`
		} `yaml:"build"`
		Test struct {
			Run string `yaml:"run"`
		} `yaml:"test"`
	}
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("produced invalid YAML: %v\n%s", err, data)
	}
	if len(out.Setup) != 1 {
		t.Fatalf("expected exactly one setup step, got %v\n%s", out.Setup, data)
	}
	if out.Setup[0].Run != "yarn install" {
		t.Errorf("setup step run = %q, want %q", out.Setup[0].Run, "yarn install")
	}
	if !out.Setup[0].RunOnce {
		t.Error("expected the detected install step to be runOnce: true")
	}
	if out.Build.Run != "yarn build" {
		t.Errorf("build task run = %q, want %q", out.Build.Run, "yarn build")
	}
	if out.Test.Run != "yarn test" {
		t.Errorf("test task run = %q, want %q", out.Test.Run, "yarn test")
	}
}

func TestWriteInitManifest_NodePlainPackageUsesNPM(t *testing.T) {
	dir := chdirTemp(t)
	writeInitTestFile(t, dir, "package.json", `{"name": "app"}`)

	if err := writeInitManifest("my-app", "node", false); err != nil {
		t.Fatalf("writeInitManifest failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "run: npm install") {
		t.Errorf("expected an npm install setup step, got:\n%s", data)
	}
	if !strings.Contains(string(data), "run: npm run build") {
		t.Errorf("expected npm run build task, got:\n%s", data)
	}
}

func TestWriteInitManifest_NodeWithoutPackageJSONHasNoSetupBlock(t *testing.T) {
	dir := chdirTemp(t)
	if err := writeInitManifest("my-app", "node", false); err != nil {
		t.Fatalf("writeInitManifest failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "setup:") {
		t.Errorf("expected no setup: block without a package.json to detect, got:\n%s", data)
	}
}

func TestWriteDevcontainer_WritesValidJSONC(t *testing.T) {
	dir := chdirTemp(t)
	if err := writeDevcontainer("go"); err != nil {
		t.Fatalf("writeDevcontainer failed: %v", err)
	}
	path := filepath.Join(dir, ".devcontainer", "devcontainer.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading devcontainer.json: %v", err)
	}
	stripped := stripJSONComments(data)
	var cfg devcontainerJSON
	if err := json.Unmarshal(stripped, &cfg); err != nil {
		t.Fatalf("devcontainer.json is not valid JSON once comments are stripped: %v\n%s", err, stripped)
	}
	if cfg.Image == "" {
		t.Error("expected a non-empty image")
	}
}

func TestWriteDevcontainer_DoesNotOverwriteExisting(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.MkdirAll(filepath.Join(dir, ".devcontainer"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".devcontainer", "devcontainer.json")
	sentinel := []byte(`{"name": "do-not-touch"}`)
	if err := os.WriteFile(path, sentinel, 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeDevcontainer("go"); err != nil {
		t.Fatalf("writeDevcontainer failed: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(sentinel) {
		t.Errorf("expected writeDevcontainer to leave an existing file untouched, got %s", got)
	}
}

// withStdin temporarily replaces os.Stdin with a pipe fed by input, for
// exercising interactive prompts (runInit) deterministically.
func withStdin(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old })
	go func() {
		_, _ = w.WriteString(input)
		w.Close()
	}()
}

func TestRunInit_AlreadyExistsErrors(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.WriteFile(filepath.Join(dir, manifestFile), []byte("version: 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runInit(nil); err == nil {
		t.Error("expected an error when devx.yaml already exists")
	}
}

func TestRunInit_DefaultsWriteManifestAndDevcontainer(t *testing.T) {
	dir := chdirTemp(t)
	// Blank lines everywhere: accept every default (project name, stack
	// dotnet, devcontainer yes, AI yes).
	withStdin(t, "\n\n\n\n")

	var initErr error
	_ = captureStdout(t, func() {
		initErr = runInit(nil)
	})
	if initErr != nil {
		t.Fatalf("runInit failed: %v", initErr)
	}

	manifestPath := filepath.Join(dir, manifestFile)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("expected %s to be written: %v", manifestFile, err)
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("devx.yaml is not valid YAML: %v\n%s", err, data)
	}
	if _, ok := doc["ai"]; !ok {
		t.Error("expected an ai: block since AI default is yes")
	}

	dcPath := filepath.Join(dir, ".devcontainer", "devcontainer.json")
	if _, err := os.Stat(dcPath); err != nil {
		t.Errorf("expected .devcontainer/devcontainer.json to be written: %v", err)
	}
}

func TestRunInit_DeclineDevcontainerAndAI(t *testing.T) {
	dir := chdirTemp(t)
	withStdin(t, "myproj\ngo\nn\nn\n")

	var initErr error
	_ = captureStdout(t, func() {
		initErr = runInit(nil)
	})
	if initErr != nil {
		t.Fatalf("runInit failed: %v", initErr)
	}

	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		t.Fatalf("expected %s to be written: %v", manifestFile, err)
	}
	if strings.Contains(string(data), "ai:") {
		t.Error("expected no ai: block when AI is declined")
	}
	if !strings.Contains(string(data), "name: myproj") {
		t.Errorf("expected project name myproj, got:\n%s", data)
	}

	dcPath := filepath.Join(dir, ".devcontainer", "devcontainer.json")
	if _, err := os.Stat(dcPath); err == nil {
		t.Error("expected no devcontainer.json to be written when declined")
	}
}
