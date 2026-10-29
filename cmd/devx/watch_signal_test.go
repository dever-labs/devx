package main

import (
	"context"
	"testing"
	"time"

	"github.com/dever-labs/devx/internal/task"
)

// TestWatchAndRerun_StopsOnContextCancel exercises watchAndRerun's main
// select loop by cancelling its context almost immediately — it should
// return nil promptly instead of blocking on the 1s ticker forever.
func TestWatchAndRerun_StopsOnContextCancel(t *testing.T) {
	writeManifest(t, `version: 1
project:
  name: my-app
api:
  run: "true"
  sources: ["*.go"]
`)
	m, err := loadValidManifest()
	if err != nil {
		t.Fatalf("loadValidManifest: %v", err)
	}
	sched, err := newScheduler(m, nil, task.Options{})
	if err != nil {
		t.Fatalf("newScheduler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before watchAndRerun's loop starts

	done := make(chan error, 1)
	go func() {
		done <- watchAndRerun(ctx, sched, m, "api")
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("watchAndRerun() = %v, want nil on context cancel", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watchAndRerun did not return promptly after context cancellation")
	}
}
