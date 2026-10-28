package main

import (
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/dever-labs/devx/internal/config"
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
