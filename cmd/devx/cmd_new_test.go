package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRunNew_AppendsTaskAndRoundTripsAsYAML(t *testing.T) {
	dir := chdirTemp(t)
	base := "version: 1\n\nbuild:\n  run: echo build\n"
	if err := os.WriteFile(filepath.Join(dir, manifestFile), []byte(base), 0644); err != nil {
		t.Fatal(err)
	}

	if err := runNew([]string{"deploy", "--run", "echo deploying", "--desc", "Deploys the app"}); err != nil {
		t.Fatalf("runNew failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]interface{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("manifest is not valid YAML after runNew: %v\n%s", err, data)
	}
	deploy, ok := out["deploy"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected a 'deploy' task block, got %v", out)
	}
	if deploy["run"] != "echo deploying" {
		t.Errorf("deploy.run = %v, want %q", deploy["run"], "echo deploying")
	}
	if deploy["desc"] != "Deploys the app" {
		t.Errorf("deploy.desc = %v, want %q", deploy["desc"], "Deploys the app")
	}
}

func TestRunNew_BareShorthandWithoutDesc(t *testing.T) {
	dir := chdirTemp(t)
	base := "version: 1\n\nbuild:\n  run: echo build\n"
	if err := os.WriteFile(filepath.Join(dir, manifestFile), []byte(base), 0644); err != nil {
		t.Fatal(err)
	}

	if err := runNew([]string{"lint"}); err != nil {
		t.Fatalf("runNew failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "lint: echo 'add your command here'") {
		t.Errorf("expected bare-string task shorthand to be appended, got:\n%s", data)
	}
}

func TestRunNew_MissingManifest(t *testing.T) {
	chdirTemp(t)
	if err := runNew([]string{"deploy"}); err == nil {
		t.Fatal("expected an error when devx.yaml does not exist")
	}
}

func TestRunNew_RequiresExactlyOneArg(t *testing.T) {
	chdirTemp(t)
	if err := runNew(nil); err == nil {
		t.Fatal("expected an error when no task name is given")
	}
	if err := runNew([]string{"a", "b"}); err == nil {
		t.Fatal("expected an error when more than one positional arg is given")
	}
}
