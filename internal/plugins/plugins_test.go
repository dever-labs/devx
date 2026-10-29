package plugins

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDiscover_EmptyPathReturnsNil(t *testing.T) {
	t.Setenv("PATH", "")
	got, err := Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got != nil {
		t.Errorf("Discover() = %+v, want nil for an empty PATH", got)
	}
}

func TestDiscover_FindsPrefixedExecutables(t *testing.T) {
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, Prefix+"echo"))
	writeExecutable(t, filepath.Join(dir, "not-a-provider"))

	t.Setenv("PATH", dir)
	got, err := Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 || filepath.Base(got[0]) != Prefix+"echo" {
		t.Errorf("Discover() = %+v, want exactly one devx-provider-echo entry", got)
	}
}

func TestDiscover_DeduplicatesAcrossPathDirs(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	writeExecutable(t, filepath.Join(dir1, Prefix+"dup"))
	writeExecutable(t, filepath.Join(dir2, Prefix+"dup"))
	writeExecutable(t, filepath.Join(dir2, Prefix+"unique"))

	sep := string(os.PathListSeparator)
	t.Setenv("PATH", dir1+sep+dir2)

	got, err := Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	names := map[string]int{}
	for _, p := range got {
		names[filepath.Base(p)]++
	}
	if names[Prefix+"dup"] != 1 {
		t.Errorf("expected devx-provider-dup to be deduplicated by name, got count %d in %+v", names[Prefix+"dup"], got)
	}
	if names[Prefix+"unique"] != 1 {
		t.Errorf("expected devx-provider-unique to be found, got %+v", got)
	}
}

func TestDiscover_SkipsUnreadablePathEntries(t *testing.T) {
	sep := string(os.PathListSeparator)
	t.Setenv("PATH", "/definitely/does/not/exist"+sep+t.TempDir())
	if _, err := Discover(); err != nil {
		t.Fatalf("Discover() should skip unreadable PATH entries without erroring, got: %v", err)
	}
}

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	mode := os.FileMode(0644)
	if runtime.GOOS != "windows" {
		mode = 0755
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho hi\n"), mode); err != nil {
		t.Fatal(err)
	}
}
