// Package testexec provides a cross-platform fake "runtime binary" for
// tests that need to intercept exec.Command("docker"/"podman"/...) calls
// without the real CLI installed.
//
// Earlier versions of these fixtures wrote "#!/bin/sh"-prefixed shell
// scripts, which silently fail to execute on Windows (no shebang-line
// interpreter dispatch) — see issue #39. This package instead copies the
// currently running test binary itself and re-execs it: the copy detects
// (via IsHelperProcess) that it's being invoked as a fake binary rather
// than as `go test`, and dispatches to RunHelperProcess instead of the
// real test suite. This works identically on every GOOS devx ships for,
// using only the Go toolchain already required to run `go test`.
package testexec

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
)

// helperProcessEnv marks a re-exec'd copy of the test binary as a fake
// runtime helper process rather than the real test suite. Set by
// NewFakeBin (scoped to the calling test via t.Setenv), checked by
// IsHelperProcess. Because TestMain checks this exactly once, before any
// test has run, setting it later via t.Setenv only affects processes
// exec'd afterward — never the already-running main test binary.
const helperProcessEnv = "DEVX_TESTEXEC_HELPER_PROCESS"

// IsHelperProcess reports whether the current process was (re-)invoked as
// a fake binary (see NewFakeBin) rather than as the normal `go test`
// entrypoint. Call this first thing in TestMain.
func IsHelperProcess() bool {
	return os.Getenv(helperProcessEnv) == "1"
}

// RunHelperProcess implements the fake binary's behavior and exits the
// process — it never returns. Call it from TestMain when IsHelperProcess
// reports true.
//
// Behavior is controlled entirely by environment variables, so the same
// re-exec'd binary can stand in for "docker", "podman", etc. with
// per-test behavior set via t.Setenv:
//   - FAKE_ARGS_FILE: if set, every argument (os.Args[1:]) is written to
//     this file, one per line (mirrors `printf '%s\n' "$@" > file`).
//   - FAKE_STDOUT_FILE: if set, that file's contents are copied to stdout.
//   - FAKE_STDOUT: if set (and FAKE_STDOUT_FILE isn't), this literal
//     string is printed to stdout, followed by a newline.
//   - FAKE_EXIT_CODE: process exit code (default 0).
func RunHelperProcess() {
	if argsFile := os.Getenv("FAKE_ARGS_FILE"); argsFile != "" {
		f, err := os.Create(argsFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		for _, a := range os.Args[1:] {
			fmt.Fprintln(f, a)
		}
		f.Close()
	}

	switch {
	case os.Getenv("FAKE_STDOUT_FILE") != "":
		f, err := os.Open(os.Getenv("FAKE_STDOUT_FILE"))
		if err == nil {
			_, _ = io.Copy(os.Stdout, f)
			f.Close()
		}
	case os.Getenv("FAKE_STDOUT") != "":
		fmt.Println(os.Getenv("FAKE_STDOUT"))
	}

	code := 0
	if v := os.Getenv("FAKE_EXIT_CODE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			code = n
		}
	}
	os.Exit(code)
}

var (
	fakeBinOnce sync.Once
	fakeBinPath string
	fakeBinErr  error
)

// NewFakeBin returns the path to a cross-platform fake binary (a copy of
// the currently running test binary) and marks the current test (via
// t.Setenv) so that invoking it re-execs into RunHelperProcess instead of
// the real test suite. Behavior is selected per-test via the FAKE_* env
// vars documented on RunHelperProcess (also set with t.Setenv).
//
// The copy itself is made once per test binary process (not per-test) and
// reused — tests in this package don't run in parallel, so a single
// shared copy on disk is safe and avoids repeatedly copying the (often
// multi-megabyte) test binary.
func NewFakeBin(t *testing.T) string {
	t.Helper()
	t.Setenv(helperProcessEnv, "1")

	fakeBinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "devx-fakebin-*")
		if err != nil {
			fakeBinErr = fmt.Errorf("creating fake binary dir: %w", err)
			return
		}
		name := "fakebin"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		dst := filepath.Join(dir, name)
		if err := copyExecutable(os.Args[0], dst); err != nil {
			fakeBinErr = err
			return
		}
		fakeBinPath = dst
	})
	if fakeBinErr != nil {
		t.Fatalf("testexec.NewFakeBin: %v", fakeBinErr)
	}
	return fakeBinPath
}

// NewFakeBinNamed behaves like NewFakeBin, but places the copy in dir
// under the literal name (plus ".exe" on Windows) instead of a shared
// cache path — for callers (like internal/task) whose code under test
// invokes a fixed binary name (e.g. "docker") looked up via PATH, rather
// than a configurable full path.
func NewFakeBinNamed(t *testing.T, dir, name string) string {
	t.Helper()
	t.Setenv(helperProcessEnv, "1")

	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dst := filepath.Join(dir, name)
	if err := copyExecutable(os.Args[0], dst); err != nil {
		t.Fatalf("testexec.NewFakeBinNamed: %v", err)
	}
	return dst
}

func copyExecutable(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading test binary %q: %w", src, err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		return fmt.Errorf("writing fake binary %q: %w", dst, err)
	}
	return nil
}
