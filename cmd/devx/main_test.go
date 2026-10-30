package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout redirects os.Stdout for the duration of fn and returns
// everything written to it — several run* commands print directly via
// fmt.Println rather than returning a string.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	w.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestPrintUsage(t *testing.T) {
	out := captureStdout(t, printUsage)
	for _, want := range []string{"devx up", "devx down", "devx run", "devx validate", "devx doctor", "devx version"} {
		if !strings.Contains(out, want) {
			t.Errorf("printUsage() output missing %q", want)
		}
	}
}

// TestBuiltinCommandNames_CoverMainDispatch is a drift guard: every
// command handled explicitly in main()'s switch (besides "version" and
// "help", which don't go through run*) must also be registered in
// builtinCommandNames, since that set is what extendedWarnings uses to
// warn when a task name would be shadowed by a built-in command.
func TestBuiltinCommandNames_CoverMainDispatch(t *testing.T) {
	dispatched := []string{
		"init", "up", "down", "status", "logs", "exec", "doctor",
		"validate", "tasks", "render", "lock", "providers", "export", "ai",
		"dev", "run", "update", "completion", "ci", "env", "auth", "new",
	}
	for _, cmd := range dispatched {
		if !builtinCommandNames[cmd] {
			t.Errorf("builtinCommandNames is missing dispatched command %q", cmd)
		}
	}
}
