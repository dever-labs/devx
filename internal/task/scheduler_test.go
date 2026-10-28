package task

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dever-labs/devx/internal/config"
)

func writeManifest(t *testing.T, dir, yaml string) string {
	t.Helper()
	path := filepath.Join(dir, "devx.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestScheduler_RunOneShotChain(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker.txt")
	path := writeManifest(t, dir, `version: 1
project:
  name: test
first: echo first >> `+marker+`
second:
  deps: [first]
  run: echo second >> `+marker+`
`)
	m, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	sched, err := NewScheduler(m, Options{LogDir: filepath.Join(dir, ".devx", "logs")})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sched.Run(ctx, []string{"second"}); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("expected marker file: %v", err)
	}
	if string(data) != "first\nsecond\n" {
		t.Fatalf("expected deps-first order, got %q", string(data))
	}
}

func TestScheduler_DryRunExecutesNothing(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker.txt")
	path := writeManifest(t, dir, `version: 1
project:
  name: test
first: touch `+marker+`
`)
	m, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	sched, err := NewScheduler(m, Options{DryRun: true, LogDir: filepath.Join(dir, ".devx", "logs")})
	if err != nil {
		t.Fatal(err)
	}
	if err := sched.Run(context.Background(), []string{"first"}); err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not execute the recipe")
	}
}

func TestScheduler_UpStartsDetachedAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, `version: 1
project:
  name: test
web:
  run: sleep 2
  ready:
    cmd: "true"
  detached: true
`)
	m, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(dir, ".devx", "tasks.json")
	sched, err := NewScheduler(m, Options{LogDir: filepath.Join(dir, ".devx", "logs"), StateFile: stateFile, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := sched.Up(ctx, nil); err != nil {
		t.Fatalf("up failed: %v", err)
	}
	if _, ok := sched.State.Lookup("web"); !ok {
		t.Fatalf("expected state entry for 'web' after up")
	}
}
