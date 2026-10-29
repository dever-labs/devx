package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSnapshotMTimes_MatchesGlobsRelativeToDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("not go"), 0644); err != nil {
		t.Fatal(err)
	}

	snap := snapshotMTimes(dir, []string{"*.go"})
	if len(snap) != 1 {
		t.Fatalf("expected 1 matched file, got %v", snap)
	}
	if _, ok := snap[filepath.Join(dir, "a.go")]; !ok {
		t.Errorf("expected a.go to be in the snapshot, got %v", snap)
	}
}

func TestSnapshotMTimes_NoMatches(t *testing.T) {
	dir := t.TempDir()
	snap := snapshotMTimes(dir, []string{"*.go"})
	if len(snap) != 0 {
		t.Errorf("expected no matches in an empty dir, got %v", snap)
	}
}

func TestChanged_DetectsAddedFile(t *testing.T) {
	a := map[string]time.Time{"x": time.Unix(1, 0)}
	b := map[string]time.Time{"x": time.Unix(1, 0), "y": time.Unix(2, 0)}
	if !changed(a, b) {
		t.Error("expected changed() to detect an added file")
	}
}

func TestChanged_DetectsUpdatedMTime(t *testing.T) {
	a := map[string]time.Time{"x": time.Unix(1, 0)}
	b := map[string]time.Time{"x": time.Unix(2, 0)}
	if !changed(a, b) {
		t.Error("expected changed() to detect an updated mtime")
	}
}

func TestChanged_NoChange(t *testing.T) {
	a := map[string]time.Time{"x": time.Unix(1, 0)}
	b := map[string]time.Time{"x": time.Unix(1, 0)}
	if changed(a, b) {
		t.Error("expected changed() to report no change for identical snapshots")
	}
}
