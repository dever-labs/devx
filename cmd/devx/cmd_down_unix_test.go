//go:build !windows

package main

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
	"time"

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
