package localai

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSetup_UnknownBackendErrors(t *testing.T) {
	err := Setup(SetupOptions{Backend: "bogus-backend"})
	if err == nil {
		t.Fatal("expected an error for an unknown backend")
	}
	if !strings.Contains(err.Error(), "unknown backend") {
		t.Errorf("error = %q, want it to mention 'unknown backend'", err)
	}
}

func TestSetup_MLXOnNonAppleSiliconErrors(t *testing.T) {
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		t.Skip("this test asserts MLX setup fails on non-Apple-Silicon hosts")
	}
	err := Setup(SetupOptions{Backend: BackendMLX})
	if err == nil {
		t.Fatal("expected an error requesting MLX setup on a non-Apple-Silicon host")
	}
	if !strings.Contains(err.Error(), "Apple Silicon") {
		t.Errorf("error = %q, want it to mention Apple Silicon", err)
	}
}

// TestRunRemoteScript_ArgsAreNotShellExpanded guards against a regression of
// the command-injection bug fixed alongside this test: a model/argument
// value containing shell metacharacters (e.g. "$(...)") must reach the
// downloaded script as a literal argv string, never be interpolated into a
// shell command line and re-evaluated.
func TestRunRemoteScript_ArgsAreNotShellExpanded(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	if _, err := exec.LookPath("dever"); err == nil {
		t.Skip("'dever' is on PATH, which would bypass the download path under test")
	}

	dir := t.TempDir()
	marker := filepath.Join(dir, "marker.txt")
	canary := filepath.Join(dir, "injected.txt")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`#!/bin/bash
echo "arg1=$1" > "` + marker + `"
`))
	}))
	defer srv.Close()

	orig := devcontainersScriptsBase
	devcontainersScriptsBase = srv.URL
	defer func() { devcontainersScriptsBase = orig }()

	maliciousArg := "$(touch " + canary + ")"
	if err := runRemoteScript("test-script", maliciousArg); err != nil {
		t.Fatalf("runRemoteScript: %v", err)
	}

	if _, err := os.Stat(canary); !os.IsNotExist(err) {
		t.Fatalf("shell metacharacters in an argument were expanded/executed: %v", canary)
	}

	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("expected marker file to be written: %v", err)
	}
	want := "arg1=" + maliciousArg + "\n"
	if string(got) != want {
		t.Errorf("marker content = %q, want %q", got, want)
	}
}
