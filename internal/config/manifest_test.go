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

// TestLoad_NamespacesIntraIncludeDeps guards against a regression where
// an included file's own deps: entries (referring to another task
// defined in that same included file) were left un-namespaced, making
// them unresolvable after merging into the root graph.
func TestLoad_NamespacesIntraIncludeDeps(t *testing.T) {
	dir := t.TempDir()
	infraDir := filepath.Join(dir, "infra")
	if err := os.MkdirAll(infraDir, 0755); err != nil {
		t.Fatal(err)
	}

	root := `version: 1
project:
  name: my-app
includes:
  infra: infra/devx.yaml
`
	infra := `version: 1
db:
  run: echo db
app:
  deps: [db]
  run: echo app
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

	app, ok := m.Tasks["infra:app"]
	if !ok {
		t.Fatalf("expected namespaced task 'infra:app', got: %v", m.Tasks)
	}
	if len(app.Deps) != 1 || app.Deps[0] != "infra:db" {
		t.Fatalf("expected infra:app deps to be namespaced to [infra:db], got %v", app.Deps)
	}
	if err := Validate(m); err != nil {
		t.Fatalf("expected merged manifest to validate cleanly, got: %v", err)
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

func TestValidate_WrongVersion(t *testing.T) {
	m := &Manifest{Version: 2, Tasks: map[string]Task{
		"a": {Name: "a", Run: "echo a"},
	}}
	if err := Validate(m); err == nil {
		t.Fatal("expected validation error when version != 1")
	}
}

func TestValidate_NoTasks(t *testing.T) {
	m := &Manifest{Version: 1, Tasks: map[string]Task{}}
	if err := Validate(m); err == nil {
		t.Fatal("expected validation error when there are no tasks")
	}
}

func TestValidate_AIBlockRequiresProviderAndModel(t *testing.T) {
	m := &Manifest{Version: 1, Tasks: map[string]Task{
		"a": {Name: "a", Run: "echo a"},
	}, AI: &AIConfig{}}
	err := Validate(m)
	if err == nil {
		t.Fatal("expected validation error for an ai: block with no provider/model")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if len(ve.Issues) != 2 {
		t.Fatalf("expected 2 issues (missing provider + model), got %v", ve.Issues)
	}
}

func TestValidate_AIBlockValid(t *testing.T) {
	m := &Manifest{Version: 1, Tasks: map[string]Task{
		"a": {Name: "a", Run: "echo a"},
	}, AI: &AIConfig{Provider: "openai", Model: "gpt-4"}}
	if err := Validate(m); err != nil {
		t.Fatalf("expected no error for a complete ai: block, got %v", err)
	}
}

func TestValidate_RestartRequiresValidValue(t *testing.T) {
	detached := true
	m := &Manifest{Version: 1, Tasks: map[string]Task{
		"a": {Name: "a", Run: "echo a", Detached: &detached, Restart: "sometimes"},
	}}
	if err := Validate(m); err == nil {
		t.Fatal("expected validation error for an invalid restart: value")
	}
}

func TestValidate_RestartOnlyValidOnDetached(t *testing.T) {
	m := &Manifest{Version: 1, Tasks: map[string]Task{
		"a": {Name: "a", Run: "echo a", Restart: "always"},
	}}
	if err := Validate(m); err == nil {
		t.Fatal("expected validation error for restart: on a non-detached task")
	}
}

func TestValidate_RestartValidOnDetached(t *testing.T) {
	detached := true
	m := &Manifest{Version: 1, Tasks: map[string]Task{
		"a": {Name: "a", Run: "echo a", Detached: &detached, Restart: "on-failure"},
	}}
	if err := Validate(m); err != nil {
		t.Fatalf("expected no error for restart: on a detached task, got %v", err)
	}
}

func TestValidate_ScopeMustBeWorkspaceOrShared(t *testing.T) {
	m := &Manifest{Version: 1, Tasks: map[string]Task{
		"a": {Name: "a", Run: "echo a", Scope: "global"},
	}}
	if err := Validate(m); err == nil {
		t.Fatal("expected validation error for an invalid scope: value")
	}
}

func TestValidate_ScopeWorkspaceIsValid(t *testing.T) {
	m := &Manifest{Version: 1, Tasks: map[string]Task{
		"a": {Name: "a", Run: "docker run --name db-${workspace}", Scope: "workspace"},
	}}
	if err := Validate(m); err != nil {
		t.Fatalf("expected no error for scope: workspace with ${workspace}, got %v", err)
	}
}

func TestValidate_ValidManifestHasNoError(t *testing.T) {
	m := &Manifest{Version: 1, Tasks: map[string]Task{
		"a": {Name: "a", Run: "echo a"},
		"b": {Name: "b", Run: "echo b", Deps: []string{"a"}},
	}}
	if err := Validate(m); err != nil {
		t.Fatalf("expected no error for a well-formed manifest, got %v", err)
	}
}
