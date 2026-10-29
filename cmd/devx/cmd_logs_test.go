package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dever-labs/devx/internal/task"
)

func TestStreamPrefixed_NonFollowPrintsAllLinesWithPrefix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "task.log")
	if err := os.WriteFile(path, []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatalf("write log fixture: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log fixture: %v", err)
	}
	defer f.Close()

	var streamErr error
	out := captureStdout(t, func() {
		streamErr = streamPrefixed(context.Background(), "mytask", f, false, path)
	})
	if streamErr != nil {
		t.Fatalf("streamPrefixed failed: %v", streamErr)
	}
	for _, want := range []string{"[mytask] line one", "[mytask] line two"} {
		if !strings.Contains(out, want) {
			t.Errorf("streamPrefixed output missing %q, got:\n%s", want, out)
		}
	}
}

func TestStreamPrefixed_EmptyFileProducesNoOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.log")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("write log fixture: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log fixture: %v", err)
	}
	defer f.Close()

	var streamErr error
	out := captureStdout(t, func() {
		streamErr = streamPrefixed(context.Background(), "mytask", f, false, path)
	})
	if streamErr != nil {
		t.Fatalf("streamPrefixed failed: %v", streamErr)
	}
	if out != "" {
		t.Errorf("expected no output for an empty log file, got:\n%s", out)
	}
}

func TestTailOne_StreamsLogFileContents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "task.log")
	if err := os.WriteFile(path, []byte("hello from task\n"), 0o644); err != nil {
		t.Fatalf("write log fixture: %v", err)
	}
	entry := task.StateEntry{LogPath: path}

	var tailErr error
	out := captureStdout(t, func() {
		tailErr = tailOne(context.Background(), "api", entry, false)
	})
	if tailErr != nil {
		t.Fatalf("tailOne failed: %v", tailErr)
	}
	if !strings.Contains(out, "[api] hello from task") {
		t.Errorf("tailOne output missing expected line, got:\n%s", out)
	}
}

func TestTailOne_MissingLogFileReturnsError(t *testing.T) {
	entry := task.StateEntry{LogPath: "/nonexistent/path/to.log"}
	if err := tailOne(context.Background(), "api", entry, false); err == nil {
		t.Error("expected an error when the log file doesn't exist")
	}
}
