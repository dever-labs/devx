package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse_BareStringTask(t *testing.T) {
	data := []byte(`version: 1
project:
  name: my-app
vars:
  PORT: "5432"
db: docker run --rm -d --name db -p ${PORT}:5432 postgres:16
`)
	m, err := Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	task, ok := m.Tasks["db"]
	if !ok {
		t.Fatalf("expected task 'db'")
	}
	if task.Run != "docker run --rm -d --name db -p ${PORT}:5432 postgres:16" {
		t.Fatalf("unexpected run: %q", task.Run)
	}
	if task.Deps != nil {
		t.Fatalf("bare string task should have no deps")
	}
}

func TestParse_ObjectTask(t *testing.T) {
	data := []byte(`version: 1
project:
  name: my-app
migrate:
  deps: [db]
  run: ./scripts/migrate.sh up
`)
	m, err := Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	task := m.Tasks["migrate"]
	if len(task.Deps) != 1 || task.Deps[0] != "db" {
		t.Fatalf("unexpected deps: %v", task.Deps)
	}
	if task.Run != "./scripts/migrate.sh up" {
		t.Fatalf("unexpected run: %q", task.Run)
	}
}

func TestLoad_ResolvesIncludesWithNamespacing(t *testing.T) {
	dir := t.TempDir()
	infraDir := filepath.Join(dir, "infra")
	if err := os.MkdirAll(infraDir, 0755); err != nil {
		t.Fatal(err)
	}

	root := `version: 1
project:
  name: my-app
vars:
  ENV: dev
includes:
  infra: infra/devx.yaml
api: ./bin/api
`
	infra := `version: 1
vars:
  BUCKET: devbucket
create-bucket: mc mb local/${BUCKET}
`
	if err := os.WriteFile(filepath.Join(dir, "devx.yaml"), []byte(root), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(infraDir, "devx.yaml"), []byte(infra), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := Load(filepath.Join(dir, "devx.yaml"))
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if _, ok := m.Tasks["api"]; !ok {
		t.Fatalf("expected root task 'api'")
	}
	nsTask, ok := m.Tasks["infra:create-bucket"]
	if !ok {
		t.Fatalf("expected namespaced task 'infra:create-bucket', got: %v", m.Tasks)
	}
	if nsTask.SourceDir != infraDir {
		t.Fatalf("expected SourceDir %q, got %q", infraDir, nsTask.SourceDir)
	}
	// Included file's own vars: override root's, root's remain as fallback.
	if nsTask.ScopeVars["BUCKET"] != "devbucket" {
		t.Fatalf("expected BUCKET=devbucket, got %q", nsTask.ScopeVars["BUCKET"])
	}
	if nsTask.ScopeVars["ENV"] != "dev" {
		t.Fatalf("expected root var ENV=dev to fall through, got %q", nsTask.ScopeVars["ENV"])
	}

	rootTask := m.Tasks["api"]
	if rootTask.SourceDir != dir {
		t.Fatalf("expected root task SourceDir %q, got %q", dir, rootTask.SourceDir)
	}
}

func TestValidate_RequiresRun(t *testing.T) {
	m := &Manifest{Version: 1, Tasks: map[string]Task{"bad": {Name: "bad"}}}
	err := Validate(m)
	if err == nil {
		t.Fatalf("expected validation error for task with no run:")
	}
}

func TestValidate_InteractiveDetachedConflict(t *testing.T) {
	detached := true
	m := &Manifest{Version: 1, Tasks: map[string]Task{
		"bad": {Name: "bad", Run: "echo hi", Interactive: true, Detached: &detached},
	}}
	if err := Validate(m); err == nil {
		t.Fatalf("expected validation error for interactive+detached")
	}
}

func TestValidate_GeneratesOnDetachedRejected(t *testing.T) {
	detached := true
	m := &Manifest{Version: 1, Tasks: map[string]Task{
		"bad": {Name: "bad", Run: "echo hi", Generates: []string{"out"}, Detached: &detached},
	}}
	if err := Validate(m); err == nil {
		t.Fatalf("expected validation error for generates: on detached task")
	}
}

func TestValidate_SharedScopeRejectsWorkspaceTokens(t *testing.T) {
	m := &Manifest{Version: 1, Tasks: map[string]Task{
		"db": {Name: "db", Run: "docker run --name db-${workspace} -d postgres", Scope: "shared"},
	}}
	if err := Validate(m); err == nil {
		t.Fatalf("expected validation error for ${workspace} on a shared task")
	}
}

func TestValidate_UnknownDep(t *testing.T) {
	m := &Manifest{Version: 1, Tasks: map[string]Task{
		"a": {Name: "a", Run: "echo a", Deps: []string{"missing"}},
	}}
	if err := Validate(m); err == nil {
		t.Fatalf("expected validation error for unknown dep")
	}
}
