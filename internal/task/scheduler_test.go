package task

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dever-labs/devx/internal/config"
)

func writeManifest(t *testing.T, dir, yaml string) string {
	t.Helper()
	path := filepath.Join(dir, "devx.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestScheduler_RunOneShotChain(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker.txt")
	// Interpolated unquoted into an `sh -c` command string below: on
	// Windows, the default shell is Git Bash's sh.exe, which treats
	// backslash as an escape character outside quotes, so a native
	// filepath.Join path (using "\") gets mangled. Forward slashes are
	// understood by both sh and the Win32 file APIs devx calls directly
	// (os.ReadFile below), so use those for the shell-visible path.
	markerSh := filepath.ToSlash(marker)
	// shell: sh is explicit here (rather than relying on the
	// platform-default shell) because the assertion below expects exact
	// LF-separated content; cmd.exe's "echo" emits CRLF, which would
	// fail that comparison even though the chain itself still ran fine.
	path := writeManifest(t, dir, `version: 1
project:
  name: test
first:
  run: echo first >> `+markerSh+`
  shell: sh
second:
  deps: [first]
  run: echo second >> `+markerSh+`
  shell: sh
`)
	m, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	sched, err := NewScheduler(m, Options{LogDir: filepath.Join(dir, ".devx", "logs")})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sched.Run(ctx, []string{"second"}); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("expected marker file: %v", err)
	}
	if string(data) != "first\nsecond\n" {
		t.Fatalf("expected deps-first order, got %q", string(data))
	}
}

func TestScheduler_DryRunExecutesNothing(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker.txt")
	markerSh := filepath.ToSlash(marker)
	path := writeManifest(t, dir, `version: 1
project:
  name: test
first: touch `+markerSh+`
`)
	m, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	sched, err := NewScheduler(m, Options{DryRun: true, LogDir: filepath.Join(dir, ".devx", "logs")})
	if err != nil {
		t.Fatal(err)
	}
	if err := sched.Run(context.Background(), []string{"first"}); err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not execute the recipe")
	}
}

func TestScheduler_UpStartsDetachedAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, `version: 1
project:
  name: test
web:
  run: sleep 2
  shell: sh
  ready:
    cmd: "exit 0"
  detached: true
`)
	m, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(dir, ".devx", "tasks.json")
	sched, err := NewScheduler(m, Options{LogDir: filepath.Join(dir, ".devx", "logs"), StateFile: stateFile, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := sched.Up(ctx, nil); err != nil {
		t.Fatalf("up failed: %v", err)
	}
	if _, ok := sched.State.Lookup("web"); !ok {
		t.Fatalf("expected state entry for 'web' after up")
	}

	// web is detached, so its process (and the goroutine that eventually
	// closes its log file) outlives Up() by design — exactly like a real
	// `devx up` invocation, which exits while the task keeps running.
	// Left alone, that means the log file handle may still be open when
	// t.TempDir() tries to remove dir's contents during cleanup, which
	// Windows (unlike POSIX) refuses to do. Stop() blocks until the
	// process has exited and (per driver.go's close-before-signal
	// ordering) its log file has been closed, so do that before the test
	// ends. See issue #39.
	if h, ok := sched.handles["web"]; ok {
		t.Cleanup(func() {
			_ = h.Stop(context.Background(), 5*time.Second)
		})
	}
}
