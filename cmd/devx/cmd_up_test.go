package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunUp_FlagAfterTaskName is a regression test for issue #59: a flag
// placed after a task name (as opposed to before it) used to be swallowed
// into the positional/task-name list by flag.FlagSet.Parse's "stop at the
// first non-flag" behavior, making `devx up api --dry-run` fail with an
// "unknown task" error instead of running "api" in dry-run mode.
func TestRunUp_FlagAfterTaskName(t *testing.T) {
	dir := chdirTemp(t)
	manifest := `version: 1
project:
  name: my-app
api:
  run: "echo hi"
`
	if err := os.WriteFile(filepath.Join(dir, "devx.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runUp(context.Background(), []string{"api", "--dry-run"}); err != nil {
		t.Errorf("runUp([]string{\"api\", \"--dry-run\"}) = %v, want nil", err)
	}
}

// TestExpandDefaultTargets_TaskNameAndGroup is a regression test for
// issue #55: default: entries can name either a task directly or a
// group: shared by several tasks, and bare `devx up` (no explicit names/
// group) should expand to exactly that set.
func TestExpandDefaultTargets_TaskNameAndGroup(t *testing.T) {
	m := mustLoad(t, `version: 1
project:
  name: my-app
default: [api, backend]
api:
  run: "echo api"
worker:
  run: "echo worker"
  group: backend
scheduler:
  run: "echo scheduler"
  group: backend
other:
  run: "echo other"
`)

	got := expandDefaultTargets(m)
	want := map[string]bool{"api": true, "worker": true, "scheduler": true}
	if len(got) != len(want) {
		t.Fatalf("expandDefaultTargets() = %v, want exactly %v", got, want)
	}
	for _, n := range got {
		if !want[n] {
			t.Errorf("expandDefaultTargets() included unexpected task %q", n)
		}
	}
}

// TestRunUp_DefaultAppliesWhenNoNamesOrGroupGiven confirms bare `devx up`
// (no task names, no --group) runs only the manifest's default: targets,
// not every task.
func TestRunUp_DefaultAppliesWhenNoNamesOrGroupGiven(t *testing.T) {
	dir := chdirTemp(t)
	manifest := `version: 1
project:
  name: my-app
default: [api]
api:
  run: "echo api"
worker:
  run: "echo worker"
`
	if err := os.WriteFile(filepath.Join(dir, "devx.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := runUp(context.Background(), []string{"--dry-run"}); err != nil {
			t.Fatalf("runUp: %v", err)
		}
	})
	if !strings.Contains(out, "[api]") {
		t.Errorf("expected the default: task %q to run, output: %s", "api", out)
	}
	if strings.Contains(out, "[worker]") {
		t.Errorf("expected only the default: target to run, not worker, output: %s", out)
	}
}

func TestRunUp_MissingManifestErrors(t *testing.T) {
	chdirTemp(t)
	if err := runUp(context.Background(), []string{"--dry-run"}); err == nil {
		t.Error("expected an error when devx.yaml doesn't exist")
	}
}
