package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

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
