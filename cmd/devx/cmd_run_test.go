package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRunTasks_HumanReadable(t *testing.T) {
	writeManifest(t, validManifest)
	out := captureStdout(t, func() {
		if err := runTasks(nil); err != nil {
			t.Fatalf("runTasks: %v", err)
		}
	})
	if !strings.Contains(out, "api") {
		t.Errorf("expected task list to mention %q, got %q", "api", out)
	}
}

func TestRunTasks_JSON(t *testing.T) {
	writeManifest(t, `version: 1
project:
  name: my-app
api:
  run: "echo hi"
  desc: "the api"
  port: 8080
`)
	out := captureStdout(t, func() {
		if err := runTasks([]string{"--json"}); err != nil {
			t.Fatalf("runTasks --json: %v", err)
		}
	})

	var tasks []map[string]any
	if err := json.Unmarshal([]byte(out), &tasks); err != nil {
		t.Fatalf("output isn't valid JSON: %v\n%s", err, out)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0]["name"] != "api" {
		t.Errorf("name = %v, want api", tasks[0]["name"])
	}
	if tasks[0]["desc"] != "the api" {
		t.Errorf("desc = %v, want 'the api'", tasks[0]["desc"])
	}
}

func TestRunTasks_NoManifest(t *testing.T) {
	chdirTemp(t)
	if err := runTasks(nil); err == nil {
		t.Fatal("expected error when devx.yaml is missing")
	}
}

func TestRunRun_UnknownTask(t *testing.T) {
	writeManifest(t, validManifest)
	if err := runRun(context.Background(), []string{"nonexistent"}); err == nil {
		t.Fatal("expected error for unknown task name")
	}
}

func TestRunRun_NoArgsListsTasks(t *testing.T) {
	writeManifest(t, validManifest)
	out := captureStdout(t, func() {
		if err := runRun(context.Background(), nil); err != nil {
			t.Fatalf("runRun with no args: %v", err)
		}
	})
	if !strings.Contains(out, "api") {
		t.Errorf("expected task list to mention %q, got %q", "api", out)
	}
}

func TestRunRun_DryRun(t *testing.T) {
	writeManifest(t, validManifest)
	if err := runRun(context.Background(), []string{"--dry-run", "api"}); err != nil {
		t.Fatalf("runRun --dry-run: %v", err)
	}
}

func TestRunRun_ExecutesOneShotTask(t *testing.T) {
	writeManifest(t, `version: 1
project:
  name: my-app
api:
  run: "true"
`)
	if err := runRun(context.Background(), []string{"api"}); err != nil {
		t.Fatalf("runRun: %v", err)
	}
}

func TestRunRun_WatchRequiresSingleTask(t *testing.T) {
	writeManifest(t, `version: 1
project:
  name: my-app
api:
  run: "true"
  sources: ["*.go"]
worker:
  run: "true"
  sources: ["*.go"]
`)
	if err := runRun(context.Background(), []string{"--watch", "api", "worker"}); err == nil {
		t.Fatal("expected error when --watch is combined with multiple task names")
	}
}

func TestRunRun_WatchRequiresSources(t *testing.T) {
	writeManifest(t, validManifest)
	if err := runRun(context.Background(), []string{"--watch", "api"}); err == nil {
		t.Fatal("expected error when --watch is used on a task with no sources:")
	}
}
