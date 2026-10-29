package main

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/dever-labs/devx/internal/config"
	"github.com/dever-labs/devx/internal/task"
)

// startGroupedProcess starts a short-lived command in its own process
// group (mirroring processDriver's Setpgid: true) and returns its PID.
// t.Cleanup kills it if the test forgot to (e.g. on failure).
func startGroupedProcess(t *testing.T, args ...string) int {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start process: %v", err)
	}
	pid := cmd.Process.Pid
	// Reap the child as soon as it exits — otherwise it lingers as a
	// zombie and syscall.Kill(pid, 0) keeps succeeding even though the
	// process is dead, exactly like devx's own driver reaps its tasks.
	waited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(waited)
	}()
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		<-waited
	})
	return pid
}

func TestStopProcessGroup_KillsRunningProcess(t *testing.T) {
	pid := startGroupedProcess(t, "sleep", "100")

	if err := stopProcessGroup(pid, 2*time.Second); err != nil {
		t.Fatalf("stopProcessGroup: %v", err)
	}

	if syscall.Kill(pid, 0) == nil {
		t.Fatal("expected process to be gone after stopProcessGroup")
	}
}

func TestStopProcessGroup_AlreadyExited(t *testing.T) {
	cmd := exec.Command("true")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start process: %v", err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Wait()

	// Give the kernel a moment to fully reap/release the pid so the
	// ESRCH path is exercised deterministically rather than racing it.
	time.Sleep(50 * time.Millisecond)

	if err := stopProcessGroup(pid, 200*time.Millisecond); err != nil {
		t.Fatalf("expected nil error for an already-exited process group, got %v", err)
	}
}

func TestStopTimeoutFor(t *testing.T) {
	m := &config.Manifest{Tasks: map[string]config.Task{
		"api":     {StopTimeout: "5s"},
		"worker":  {StopTimeout: "not-a-duration"},
		"nottout": {},
	}}

	cases := []struct {
		name string
		def  time.Duration
		want time.Duration
	}{
		{"api", 10 * time.Second, 5 * time.Second},
		{"worker", 10 * time.Second, 10 * time.Second}, // unparsable -> fallback
		{"nottout", 10 * time.Second, 10 * time.Second}, // unset -> fallback
		{"missing", 10 * time.Second, 10 * time.Second}, // task gone -> fallback
	}
	for _, tc := range cases {
		if got := stopTimeoutFor(m, tc.name, tc.def); got != tc.want {
			t.Errorf("stopTimeoutFor(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestReverseDependencyOrder_DependentsBeforeDependencies(t *testing.T) {
	m := &config.Manifest{Tasks: map[string]config.Task{
		"db":  {Name: "db", Run: "echo db"},
		"api": {Name: "api", Run: "echo api", Deps: []string{"db"}},
		"web": {Name: "web", Run: "echo web", Deps: []string{"api"}},
	}}
	order, err := reverseDependencyOrder(m)
	if err != nil {
		t.Fatalf("reverseDependencyOrder failed: %v", err)
	}
	if len(order) != 3 {
		t.Fatalf("expected 3 tasks, got %v", order)
	}
	pos := map[string]int{}
	for i, n := range order {
		pos[n] = i
	}
	if pos["web"] > pos["api"] {
		t.Errorf("expected web (dependent) to stop before api (dependency), got order %v", order)
	}
	if pos["api"] > pos["db"] {
		t.Errorf("expected api (dependent) to stop before db (dependency), got order %v", order)
	}
}

func TestStopEntry_NoPIDAndNotContainerIsNoop(t *testing.T) {
	entry := task.StateEntry{IsContainer: false, PID: 0}
	if err := stopEntry(context.Background(), entry, time.Second); err != nil {
		t.Errorf("expected nil error for an entry with no PID and not a container, got %v", err)
	}
}

func TestStopEntry_ContainerWithoutNameErrors(t *testing.T) {
	entry := task.StateEntry{IsContainer: true, ContainerName: ""}
	if err := stopEntry(context.Background(), entry, time.Second); err == nil {
		t.Error("expected an error when a container entry has no containerName")
	}
}

func TestStopEntry_ProcessDispatchesToProcessGroup(t *testing.T) {
	pid := startGroupedProcess(t, "sleep", "100")
	entry := task.StateEntry{IsContainer: false, PID: pid}
	if err := stopEntry(context.Background(), entry, 2*time.Second); err != nil {
		t.Fatalf("stopEntry: %v", err)
	}
	if syscall.Kill(pid, 0) == nil {
		t.Fatal("expected process to be gone after stopEntry")
	}
}

func TestStopContainerByName_EmptyNameErrors(t *testing.T) {
	if err := stopContainerByName(context.Background(), "", time.Second); err == nil {
		t.Error("expected an error when containerName is empty")
	}
}

func TestRunDown_MissingManifestErrors(t *testing.T) {
	chdirTemp(t)
	if err := runDown(context.Background(), nil); err == nil {
		t.Error("expected an error when devx.yaml doesn't exist")
	}
}

func TestReverseDependencyOrder_CycleReturnsError(t *testing.T) {
	m := &config.Manifest{Tasks: map[string]config.Task{
		"a": {Name: "a", Run: "echo a", Deps: []string{"b"}},
		"b": {Name: "b", Run: "echo b", Deps: []string{"a"}},
	}}
	if _, err := reverseDependencyOrder(m); err == nil {
		t.Fatal("expected an error for a cyclic dependency graph")
	}
}
