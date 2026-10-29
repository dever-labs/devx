package setup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dever-labs/devx/internal/config"
)

func TestRunEnvFiles_WritesFromDefaultsNonInteractive(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "config.tmpl", "db_password={{.DB_PASSWORD}}\napi_url={{.API_URL}}\n")

	files := []config.EnvFile{{
		Name:     "app config",
		Template: "config.tmpl",
		Output:   "app-config.local.yaml",
		Vars: []config.EnvVar{
			{Name: "DB_PASSWORD", Default: "devpassword"},
			{Name: "API_URL", Default: "http://localhost:8080"},
		},
	}}

	results := RunEnvFiles(dir, files, Options{})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "written" {
		t.Fatalf("expected status written, got %+v", results[0])
	}

	out, err := os.ReadFile(filepath.Join(dir, "app-config.local.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "db_password=devpassword") {
		t.Errorf("rendered output missing default DB_PASSWORD: %s", out)
	}
	if !strings.Contains(string(out), "api_url=http://localhost:8080") {
		t.Errorf("rendered output missing default API_URL: %s", out)
	}
}

func TestRunEnvFiles_EnvVarWinsOverDefault(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "config.tmpl", "db_password={{.DB_PASSWORD}}\n")
	t.Setenv("DB_PASSWORD", "from-env")

	files := []config.EnvFile{{
		Template: "config.tmpl",
		Output:   "out.yaml",
		Vars:     []config.EnvVar{{Name: "DB_PASSWORD", Default: "devpassword"}},
	}}

	RunEnvFiles(dir, files, Options{})

	out, err := os.ReadFile(filepath.Join(dir, "out.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "db_password=from-env") {
		t.Errorf("expected env var to win over default, got: %s", out)
	}
}

func TestRunEnvFiles_SkipsWhenOutputAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "config.tmpl", "db_password={{.DB_PASSWORD}}\n")
	writeTestFile(t, dir, "out.yaml", "db_password=already-here\n")

	files := []config.EnvFile{{
		Template: "config.tmpl",
		Output:   "out.yaml",
		Vars:     []config.EnvVar{{Name: "DB_PASSWORD", Default: "devpassword"}},
	}}

	results := RunEnvFiles(dir, files, Options{})
	if results[0].Status != "skipped" {
		t.Fatalf("expected status skipped, got %+v", results[0])
	}

	out, err := os.ReadFile(filepath.Join(dir, "out.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "db_password=already-here\n" {
		t.Errorf("expected existing output untouched, got: %s", out)
	}
}

func TestRunEnvFiles_NoValueAvailableFails(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "config.tmpl", "db_password={{.DB_PASSWORD}}\n")

	files := []config.EnvFile{{
		Template: "config.tmpl",
		Output:   "out.yaml",
		Vars:     []config.EnvVar{{Name: "DB_PASSWORD"}}, // no env var, no default, not interactive
	}}

	results := RunEnvFiles(dir, files, Options{})
	if results[0].Status != "failed" {
		t.Fatalf("expected status failed, got %+v", results[0])
	}
}

func TestRunEnvFiles_InteractivePromptUsed(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "config.tmpl", "db_password={{.DB_PASSWORD}}\n")

	files := []config.EnvFile{{
		Template: "config.tmpl",
		Output:   "out.yaml",
		Vars:     []config.EnvVar{{Name: "DB_PASSWORD", Prompt: "Database password"}},
	}}

	opts := Options{
		Interactive: true,
		Stdin:       strings.NewReader("typed-secret\n"),
		Stdout:      &bytes.Buffer{},
	}
	results := RunEnvFiles(dir, files, opts)
	if results[0].Status != "written" {
		t.Fatalf("expected status written, got %+v", results[0])
	}

	out, err := os.ReadFile(filepath.Join(dir, "out.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "db_password=typed-secret") {
		t.Errorf("expected prompted value in output, got: %s", out)
	}
}

func TestRunEnvFiles_InteractiveBlankLineFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "config.tmpl", "api_url={{.API_URL}}\n")

	files := []config.EnvFile{{
		Template: "config.tmpl",
		Output:   "out.yaml",
		Vars:     []config.EnvVar{{Name: "API_URL", Default: "http://localhost:8080"}},
	}}

	opts := Options{
		Interactive: true,
		Stdin:       strings.NewReader("\n"), // blank -> fall back to default
		Stdout:      &bytes.Buffer{},
	}
	RunEnvFiles(dir, files, opts)

	out, err := os.ReadFile(filepath.Join(dir, "out.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "api_url=http://localhost:8080") {
		t.Errorf("expected default fallback in output, got: %s", out)
	}
}

func TestRunEnvFiles_MissingTemplateFileFails(t *testing.T) {
	dir := t.TempDir()

	files := []config.EnvFile{{Template: "does-not-exist.tmpl", Output: "out.yaml"}}

	results := RunEnvFiles(dir, files, Options{})
	if results[0].Status != "failed" {
		t.Fatalf("expected status failed for missing template, got %+v", results[0])
	}
}

func TestRunEnvFiles_MissingKeyInTemplateFails(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "config.tmpl", "unknown={{.TYPO}}\n")

	files := []config.EnvFile{{
		Template: "config.tmpl",
		Output:   "out.yaml",
		Vars:     []config.EnvVar{{Name: "DB_PASSWORD", Default: "x"}}, // TYPO never declared
	}}

	results := RunEnvFiles(dir, files, Options{})
	if results[0].Status != "failed" {
		t.Fatalf("expected status failed for a template referencing an undeclared var, got %+v", results[0])
	}
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
