package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dever-labs/devx/internal/config"
	"github.com/dever-labs/devx/internal/task"
)

// watchAndRerun implements `devx run <name> --watch` via periodic mtime
// polling (a simplified Phase 1 stand-in for a real file-notification
// watcher — see docs/design/task-executor-refactor.md's documented
// deferral of fsnotify). Re-runs the task whenever any sources: glob
// match's mtime advances past the last observed snapshot.
func watchAndRerun(ctx context.Context, sched *task.Scheduler, m *config.Manifest, name string) error {
	t := m.Tasks[name]
	last := snapshotMTimes(t.SourceDir, t.Sources)

	fmt.Printf("[%s] watching %v for changes (Ctrl-C to stop)...\n", name, t.Sources)
	if err := sched.Run(ctx, []string{name}); err != nil {
		fmt.Printf("[%s] rebuild failed: %v\n", name, err)
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			current := snapshotMTimes(t.SourceDir, t.Sources)
			if changed(last, current) {
				fmt.Printf("[%s] change detected, re-running...\n", name)
				if err := sched.Run(ctx, []string{name}); err != nil {
					fmt.Printf("[%s] rebuild failed: %v\n", name, err)
				}
				last = current
			}
		}
	}
}

func snapshotMTimes(dir string, globs []string) map[string]time.Time {
	out := map[string]time.Time{}
	for _, pattern := range globs {
		full := pattern
		if !filepath.IsAbs(full) {
			full = filepath.Join(dir, pattern)
		}
		matches, err := filepath.Glob(full)
		if err != nil {
			continue
		}
		for _, m := range matches {
			if info, err := os.Stat(m); err == nil {
				out[m] = info.ModTime()
			}
		}
	}
	return out
}

func changed(a, b map[string]time.Time) bool {
	if len(a) != len(b) {
		return true
	}
	for k, v := range b {
		if av, ok := a[k]; !ok || !av.Equal(v) {
			return true
		}
	}
	return false
}
