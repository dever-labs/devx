package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStderr redirects os.Stderr for the duration of fn and returns
// everything written to it — the mirror of captureStdout, needed for
// runEnvFilesBestEffort's "failed" status which deliberately prints to
// stderr rather than stdout.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()

	w.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestRunEnvFilesBestEffort_NoEnvFilesIsNoop confirms a manifest with no
// envFiles: block does nothing (no files touched, nothing printed).
func TestRunEnvFilesBestEffort_NoEnvFilesIsNoop(t *testing.T) {
	m := mustLoad(t, `version: 1
project:
  name: my-app
api:
  run: "echo hi"
`)
	out := captureStdout(t, func() { runEnvFilesBestEffort(m) })
	if out != "" {
		t.Errorf("expected no output for a manifest without envFiles:, got %q", out)
	}
}

// TestRunEnvFilesBestEffort_WritesNewFile is a regression test for issue
// #61: `devx up` should scaffold a missing envFiles: output as an
// implicit, non-interactive step and report it on stdout.
func TestRunEnvFilesBestEffort_WritesNewFile(t *testing.T) {
	m := mustLoad(t, `version: 1
project:
  name: my-app
envFiles:
  - template: .env.tmpl
    output: .env
api:
  run: "echo hi"
`)
	// mustLoad already chdir'd into its own temp dir; write the template
	// there (m.Dir()) rather than into a separate directory.
	if err := os.WriteFile(filepath.Join(m.Dir(), ".env.tmpl"), []byte("KEY=value\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() { runEnvFilesBestEffort(m) })
	if !strings.Contains(out, "[envFiles]") {
		t.Errorf("expected a [envFiles] status line on stdout, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(m.Dir(), ".env")); err != nil {
		t.Errorf("expected .env to have been written, got %v", err)
	}
}

// TestRunEnvFilesBestEffort_FailureGoesToStderr confirms a scaffolding
// failure (e.g. a missing template) is reported on stderr with guidance
// to run `devx init`, and never fails devx up itself (no panic/return
// value to check — runEnvFilesBestEffort returns nothing).
func TestRunEnvFilesBestEffort_FailureGoesToStderr(t *testing.T) {
	m := mustLoad(t, `version: 1
project:
  name: my-app
envFiles:
  - template: missing.tmpl
    output: .env
api:
  run: "echo hi"
`)

	errOut := captureStderr(t, func() { runEnvFilesBestEffort(m) })
	if !strings.Contains(errOut, "[envFiles]") || !strings.Contains(errOut, "devx init") {
		t.Errorf("expected a [envFiles] failure line pointing at `devx init` on stderr, got %q", errOut)
	}
}
