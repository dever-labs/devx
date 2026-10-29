package task

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dever-labs/devx/internal/config"
)

func newTestScheduler(t *testing.T, yaml string) *Scheduler {
	t.Helper()
	dir := t.TempDir()
	path := writeManifest(t, dir, yaml)
	m, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	sched, err := NewScheduler(m, Options{LogDir: filepath.Join(dir, ".devx", "logs"), StateFile: filepath.Join(dir, ".devx", "state.json")})
	if err != nil {
		t.Fatal(err)
	}
	return sched
}

func TestScheduler_ResolveOne_Success(t *testing.T) {
	sched := newTestScheduler(t, `version: 1
project:
  name: test
build: echo hello
`)
	et, err := sched.ResolveOne("build")
	if err != nil {
		t.Fatalf("ResolveOne: %v", err)
	}
	if et.Name != "build" || et.ResolvedRun != "echo hello" {
		t.Errorf("ResolveOne() = %+v, want name=build run='echo hello'", et)
	}
}

func TestScheduler_ResolveOne_UnknownTaskErrors(t *testing.T) {
	sched := newTestScheduler(t, `version: 1
project:
  name: test
build: echo hello
`)
	if _, err := sched.ResolveOne("does-not-exist"); err == nil {
		t.Fatal("expected an error for an unknown task name")
	}
}

func TestScheduler_ResolveAll_ResolvesEveryTask(t *testing.T) {
	sched := newTestScheduler(t, `version: 1
project:
  name: test
build: echo build
test:
  deps: [build]
  run: echo test
`)
	all, err := sched.ResolveAll()
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 resolved tasks, got %d", len(all))
	}
	if all["build"] == nil || all["test"] == nil {
		t.Errorf("ResolveAll() = %+v, missing expected task names", all)
	}
}

func TestScheduler_ProbeExisting_ContainerRunning(t *testing.T) {
	dir := fakeRuntimeBin(t, "docker", withStdout("running"))
	withFakePath(t, dir)

	sched := newTestScheduler(t, `version: 1
project:
  name: test
build: echo hello
`)
	et, err := sched.ResolveOne("build")
	if err != nil {
		t.Fatal(err)
	}
	alive := sched.probeExisting(context.Background(), et, StateEntry{IsContainer: true, ContainerName: "foo"})
	if !alive {
		t.Error("expected probeExisting to report the container as alive")
	}
}

func TestScheduler_ProbeExisting_ContainerNotRunning(t *testing.T) {
	dir := fakeRuntimeBin(t, "docker", withStdout("exited"))
	withFakePath(t, dir)

	sched := newTestScheduler(t, `version: 1
project:
  name: test
build: echo hello
`)
	et, err := sched.ResolveOne("build")
	if err != nil {
		t.Fatal(err)
	}
	alive := sched.probeExisting(context.Background(), et, StateEntry{IsContainer: true, ContainerName: "foo"})
	if alive {
		t.Error("expected probeExisting to report the container as not alive")
	}
}

func TestScheduler_ProbeExisting_ProcessDeadByPID(t *testing.T) {
	sched := newTestScheduler(t, `version: 1
project:
  name: test
build: echo hello
`)
	et, err := sched.ResolveOne("build")
	if err != nil {
		t.Fatal(err)
	}
	// PID 2^30 is astronomically unlikely to be a live process.
	alive := sched.probeExisting(context.Background(), et, StateEntry{PID: 1 << 30})
	if alive {
		t.Error("expected probeExisting to report a nonexistent PID as not alive")
	}
}

func TestScheduler_ProbeExisting_ProcessAliveNoReadyCheckSucceeds(t *testing.T) {
	sched := newTestScheduler(t, `version: 1
project:
  name: test
build: echo hello
`)
	et, err := sched.ResolveOne("build")
	if err != nil {
		t.Fatal(err)
	}
	alive := sched.probeExisting(context.Background(), et, StateEntry{PID: os.Getpid()})
	if !alive {
		t.Error("expected probeExisting to report the current process (no ready check) as alive")
	}
}

func TestProcessAlive_CurrentProcessIsAlive(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Error("expected the current test process to be reported alive")
	}
}

func TestProcessAlive_NonexistentPIDIsNotAlive(t *testing.T) {
	if processAlive(1 << 30) {
		t.Error("expected an implausible PID to be reported not alive")
	}
}

func TestSanitize_ReplacesColonsAndSlashes(t *testing.T) {
	if got := sanitize("group:sub/task"); got != "group_sub_task" {
		t.Errorf("sanitize() = %q, want group_sub_task", got)
	}
}

func TestTaskTimeout_UsesExplicitOverFallback(t *testing.T) {
	et := &EffectiveTask{}
	et.Timeout = "2s"
	if got := taskTimeout(et, 10*time.Second); got != 2*time.Second {
		t.Errorf("taskTimeout() = %v, want 2s", got)
	}
}

func TestTaskTimeout_FallsBackOnInvalidOrEmpty(t *testing.T) {
	et := &EffectiveTask{}
	if got := taskTimeout(et, 10*time.Second); got != 10*time.Second {
		t.Errorf("taskTimeout(empty) = %v, want fallback 10s", got)
	}
	et.Timeout = "not-a-duration"
	if got := taskTimeout(et, 10*time.Second); got != 10*time.Second {
		t.Errorf("taskTimeout(invalid) = %v, want fallback 10s", got)
	}
}
