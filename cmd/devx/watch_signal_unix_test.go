//go:build !windows

package main

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/dever-labs/devx/internal/task"
)

// TestAttachAndWaitForInterrupt_StopsOnSignal verifies that
// attachAndWaitForInterrupt returns once it receives SIGINT, running
// runDown afterward (which may itself error since nothing is actually
// up in this temp-dir manifest, but must not hang or panic).
//
// Unix-only: sends itself a real SIGINT via syscall.Kill, which has no
// Windows equivalent (os.Process.Signal(os.Interrupt) behaves
// differently there and isn't a reliable substitute for this test).
func TestAttachAndWaitForInterrupt_StopsOnSignal(t *testing.T) {
	writeManifest(t, validManifest)
	m, err := loadValidManifest()
	if err != nil {
		t.Fatalf("loadValidManifest: %v", err)
	}
	sched, err := newScheduler(m, nil, task.Options{})
	if err != nil {
		t.Fatalf("newScheduler: %v", err)
	}

	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	done := make(chan error, 1)
	go func() {
		done <- attachAndWaitForInterrupt(context.Background(), sched)
	}()

	select {
	case <-done:
		// returned — error value is irrelevant here (runDown may fail
		// since nothing is actually up), we only care it didn't hang.
	case <-time.After(5 * time.Second):
		t.Fatal("attachAndWaitForInterrupt did not return after SIGINT")
	}
}
