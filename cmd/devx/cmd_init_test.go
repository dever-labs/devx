package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dever-labs/devx/internal/testexec"
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

func TestWriteInitManifest_NvmrcWithoutManagerFallsBackToOSPackageManager(t *testing.T) {
	// No version manager and no corepack on PATH -> deterministic fallback.
	// Also override HOME: hosted CI runners (e.g. GitHub Actions'
	// ubuntu-latest image) ship nvm pre-installed under $HOME/.nvm, which
	// would otherwise make NodeVersionManager() find it regardless of PATH.
	t.Setenv("PATH", t.TempDir())
	t.Setenv("NVM_DIR", "")
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir) // os.UserHomeDir() uses this on Windows

	dir := chdirTemp(t)
	writeInitTestFile(t, dir, ".nvmrc", "20.11.0")
	writeInitTestFile(t, dir, "package.json", `{"name": "app"}`)

	if err := writeInitManifest("my-app", "node", false); err != nil {
		t.Fatalf("writeInitManifest failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Tools []struct {
			Name    string `yaml:"name"`
			Version string `yaml:"version"`
			Install struct {
				MacOS string `yaml:"macos"`
			} `yaml:"install"`
		} `yaml:"tools"`
	}
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("produced invalid YAML: %v\n%s", err, data)
	}
	if len(out.Tools) != 1 || out.Tools[0].Name != "node" {
		t.Fatalf("expected a single node tools entry, got %v\n%s", out.Tools, data)
	}
	if out.Tools[0].Version != "20.11.0" {
		t.Errorf("tools[0].version = %q, want %q", out.Tools[0].Version, "20.11.0")
	}
	if out.Tools[0].Install.MacOS != "brew install node@20.11.0" {
		t.Errorf("tools[0].install.macos = %q, want OS-package-manager fallback", out.Tools[0].Install.MacOS)
	}
}

func TestWriteInitManifest_NodeVersionFileUsesVersionManagerWhenPresent(t *testing.T) {
	// Put a fake "fnm" binary on PATH so NodeVersionManager() finds it.
	// NodeVersionManager only calls exec.LookPath (never executes the
	// binary), but LookPath still requires a real, correctly-named
	// executable file -- a bare "#!/bin/sh" script has no shebang
	// dispatch on Windows and, lacking a PATHEXT-registered extension,
	// wouldn't even be found there. testexec.NewFakeBinNamed produces a
	// binary that's valid (and found) on every platform.
	binDir := t.TempDir()
	testexec.NewFakeBinNamed(t, binDir, "fnm")
	t.Setenv("PATH", binDir)

	dir := chdirTemp(t)
	writeInitTestFile(t, dir, ".node-version", "22.11.0")

	if err := writeInitManifest("my-app", "node", false); err != nil {
		t.Fatalf("writeInitManifest failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "fnm install 22.11.0 && fnm use 22.11.0") {
		t.Errorf("expected fnm-based install command, got:\n%s", data)
	}
}

func TestWriteInitManifest_NoVersionPinHasNoToolsBlock(t *testing.T) {
	dir := chdirTemp(t)
	writeInitTestFile(t, dir, "package.json", `{"name": "app"}`)

	if err := writeInitManifest("my-app", "node", false); err != nil {
		t.Fatalf("writeInitManifest failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "tools:") {
		t.Errorf("expected no tools: block without a detected version pin, got:\n%s", data)
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

func TestWriteLaunchJSON_NodeAndGoProduceValidJSON(t *testing.T) {
	for _, stack := range []string{"node", "go"} {
		t.Run(stack, func(t *testing.T) {
			dir := chdirTemp(t)
			ok, err := writeLaunchJSON(stack)
			if err != nil {
				t.Fatalf("writeLaunchJSON(%q) failed: %v", stack, err)
			}
			if !ok {
				t.Fatalf("writeLaunchJSON(%q) = false, want true", stack)
			}
			data, err := os.ReadFile(filepath.Join(dir, ".vscode", "launch.json"))
			if err != nil {
				t.Fatalf("reading launch.json: %v", err)
			}
			var cfg struct {
				Configurations []map[string]interface{} `json:"configurations"`
			}
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Fatalf("launch.json is not valid JSON: %v\n%s", err, data)
			}
			if len(cfg.Configurations) == 0 {
				t.Error("expected at least one configuration")
			}
		})
	}
}

func TestWriteLaunchJSON_UnsupportedStackIsNoopNotError(t *testing.T) {
	chdirTemp(t)
	for _, stack := range []string{"dotnet", "python", "other"} {
		ok, err := writeLaunchJSON(stack)
		if err != nil {
			t.Fatalf("writeLaunchJSON(%q) returned an error, want nil: %v", stack, err)
		}
		if ok {
			t.Errorf("writeLaunchJSON(%q) = true, want false (not templated)", stack)
		}
		if _, err := os.Stat(".vscode/launch.json"); err == nil {
			t.Errorf("writeLaunchJSON(%q) should not have written a file", stack)
		}
	}
}

func TestWriteLaunchJSON_DoesNotOverwriteExisting(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.MkdirAll(filepath.Join(dir, ".vscode"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".vscode", "launch.json")
	sentinel := []byte(`{"configurations": []}`)
	if err := os.WriteFile(path, sentinel, 0644); err != nil {
		t.Fatal(err)
	}
	ok, err := writeLaunchJSON("node")
	if err != nil {
		t.Fatalf("writeLaunchJSON failed: %v", err)
	}
	if ok {
		t.Error("expected writeLaunchJSON to report false when launch.json already exists")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(sentinel) {
		t.Errorf("expected writeLaunchJSON to leave an existing file untouched, got %s", got)
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

func TestRunInit_SkipsWizardWhenManifestExists(t *testing.T) {
	dir := chdirTemp(t)
	manifest := "version: 1\nproject:\n  name: existing\n"
	if err := os.WriteFile(filepath.Join(dir, manifestFile), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	var initErr error
	out := captureStdout(t, func() {
		initErr = runInit(context.Background(), nil)
	})
	if initErr != nil {
		t.Fatalf("runInit failed: %v", initErr)
	}
	if strings.Contains(out, "devx init — set up this project") {
		t.Error("expected the interactive wizard to be skipped when devx.yaml already exists")
	}
	if !strings.Contains(out, "Nothing to do") {
		t.Errorf("expected runInit to run project setup directly when devx.yaml already exists, got:\n%s", out)
	}
}

func TestRunInit_DefaultsWriteManifestAndDevcontainer(t *testing.T) {
	dir := chdirTemp(t)
	// Blank lines everywhere: accept every default (project name, stack
	// dotnet, devcontainer yes, AI yes).
	withStdin(t, "\n\n\n\n\n")

	var initErr error
	_ = captureStdout(t, func() {
		initErr = runInit(context.Background(), nil)
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
	withStdin(t, "myproj\ngo\nn\nn\nn\n")

	var initErr error
	_ = captureStdout(t, func() {
		initErr = runInit(context.Background(), nil)
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

func TestRunInit_AcceptsLaunchJSONForGoStack(t *testing.T) {
	dir := chdirTemp(t)
	// name, stack=3 (go), devcontainer=n, launchjson=y, ai=n
	withStdin(t, "myproj\n3\nn\ny\nn\n")

	var initErr error
	_ = captureStdout(t, func() {
		initErr = runInit(context.Background(), nil)
	})
	if initErr != nil {
		t.Fatalf("runInit failed: %v", initErr)
	}

	ljPath := filepath.Join(dir, ".vscode", "launch.json")
	if _, err := os.Stat(ljPath); err != nil {
		t.Errorf("expected .vscode/launch.json to be written: %v", err)
	}
}
