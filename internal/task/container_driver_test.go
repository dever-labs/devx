package task

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dever-labs/devx/internal/testexec"
)

// fakeBinOpt configures the behavior of a fakeRuntimeBin instance; see
// withExitCode, withStdout, and withArgsFile.
type fakeBinOpt func(t *testing.T)

// withExitCode makes the fake runtime binary exit with the given code
// instead of the default 0.
func withExitCode(code int) fakeBinOpt {
	return func(t *testing.T) { t.Setenv("FAKE_EXIT_CODE", strconv.Itoa(code)) }
}

// withStdout makes the fake runtime binary print s (plus a trailing
// newline) to stdout.
func withStdout(s string) fakeBinOpt {
	return func(t *testing.T) { t.Setenv("FAKE_STDOUT", s) }
}

// withArgsFile makes the fake runtime binary record every argument it
// receives (one per line) to path.
func withArgsFile(path string) fakeBinOpt {
	return func(t *testing.T) { t.Setenv("FAKE_ARGS_FILE", path) }
}

// fakeRuntimeBin writes a cross-platform fake binary (see
// internal/testexec) named "docker" or "podman" into a fresh temp dir,
// returning that dir. Prepending it to PATH lets containerHandle's
// exec.CommandContext calls (which always invoke the literal
// "docker"/"podman" binary name) be intercepted deterministically,
// without needing the real CLI. opts configure the fake binary's
// behavior via the FAKE_* environment variables documented in
// internal/testexec.
func fakeRuntimeBin(t *testing.T, name string, opts ...fakeBinOpt) string {
	t.Helper()
	dir := t.TempDir()
	for _, opt := range opts {
		opt(t)
	}
	testexec.NewFakeBinNamed(t, dir, name)
	return dir
}

func withFakePath(t *testing.T, dirs ...string) {
	t.Helper()
	t.Setenv("PATH", strings.Join(dirs, string(os.PathListSeparator))+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestContainerDriver_Start_Success(t *testing.T) {
	dir := fakeRuntimeBin(t, "docker")
	withFakePath(t, dir)

	d := NewContainerDriver()
	et := newEffectiveTask("t", "docker run -d --name foo image", ".")
	et.ContainerName = "foo"

	h, err := d.Start(context.Background(), et, filepath.Join(t.TempDir(), "task.log"))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	ch, ok := h.(*containerHandle)
	if !ok {
		t.Fatalf("expected *containerHandle, got %T", h)
	}
	if ch.name != "foo" || ch.runtime != "docker" {
		t.Errorf("containerHandle = %+v, want name=foo runtime=docker", ch)
	}
}

func TestContainerDriver_Start_DetectsPodmanRuntime(t *testing.T) {
	dir := fakeRuntimeBin(t, "podman")
	withFakePath(t, dir)

	d := NewContainerDriver()
	et := newEffectiveTask("t", "podman run -d --name foo image", ".")
	et.ContainerName = "foo"

	h, err := d.Start(context.Background(), et, filepath.Join(t.TempDir(), "task.log"))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	ch := h.(*containerHandle)
	if ch.runtime != "podman" {
		t.Errorf("runtime = %q, want podman", ch.runtime)
	}
}

func TestContainerDriver_Start_MissingContainerNameErrors(t *testing.T) {
	d := NewContainerDriver()
	et := newEffectiveTask("t", "true", ".")
	// ContainerName left empty.

	_, err := d.Start(context.Background(), et, filepath.Join(t.TempDir(), "task.log"))
	if err == nil {
		t.Fatal("expected an error when ContainerName can't be determined")
	}
}

func TestContainerDriver_Start_CommandFailureErrors(t *testing.T) {
	d := NewContainerDriver()
	et := newEffectiveTask("t", "false", ".")
	et.ContainerName = "foo"

	_, err := d.Start(context.Background(), et, filepath.Join(t.TempDir(), "task.log"))
	if err == nil {
		t.Fatal("expected an error when the run: command fails")
	}
}

func TestContainerHandle_Status_Running(t *testing.T) {
	dir := fakeRuntimeBin(t, "docker", withStdout("running"))
	withFakePath(t, dir)

	h := &containerHandle{name: "foo", runtime: "docker"}
	status, err := h.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status != "running" {
		t.Errorf("status = %q, want running", status)
	}
}

func TestContainerHandle_Status_OtherState(t *testing.T) {
	dir := fakeRuntimeBin(t, "docker", withStdout("exited"))
	withFakePath(t, dir)

	h := &containerHandle{name: "foo", runtime: "docker"}
	status, err := h.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status != "exited" {
		t.Errorf("status = %q, want exited", status)
	}
}

func TestContainerHandle_Status_CommandFailureReturnsExited(t *testing.T) {
	dir := fakeRuntimeBin(t, "docker", withExitCode(1))
	withFakePath(t, dir)

	h := &containerHandle{name: "foo", runtime: "docker"}
	status, err := h.Status(context.Background())
	if err != nil {
		t.Fatalf("Status should swallow inspect errors, got: %v", err)
	}
	if status != "exited" {
		t.Errorf("status = %q, want exited for a failed inspect", status)
	}
}

func TestContainerHandle_Stop_PassesTimeoutSeconds(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	dir := fakeRuntimeBin(t, "docker", withArgsFile(argsFile))
	withFakePath(t, dir)

	h := &containerHandle{name: "foo", runtime: "docker"}
	if err := h.Stop(context.Background(), 5*time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("reading args file: %v", err)
	}
	args := string(data)
	if !strings.Contains(args, "stop") || !strings.Contains(args, "-t") || !strings.Contains(args, "5") || !strings.Contains(args, "foo") {
		t.Errorf("Stop() args = %q, want stop -t 5 foo", args)
	}
}

func TestContainerHandle_Logs_Follow(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	dir := fakeRuntimeBin(t, "docker", withArgsFile(argsFile), withStdout("container-log-line"))
	withFakePath(t, dir)

	h := &containerHandle{name: "foo", runtime: "docker"}
	rc, err := h.Logs(context.Background(), true)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !strings.Contains(string(data), "container-log-line") {
		t.Errorf("Logs() output = %q, want to contain container-log-line", data)
	}

	argsData, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("reading args file: %v", err)
	}
	if !strings.Contains(string(argsData), "-f") {
		t.Errorf("Logs(follow=true) args = %q, want -f flag", argsData)
	}
}

func TestContainerHandle_Logs_NoFollow(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	dir := fakeRuntimeBin(t, "docker", withArgsFile(argsFile))
	withFakePath(t, dir)

	h := &containerHandle{name: "foo", runtime: "docker"}
	rc, err := h.Logs(context.Background(), false)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	defer rc.Close()
	_, _ = io.ReadAll(rc)

	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("reading args file: %v", err)
	}
	if strings.Contains(string(data), "-f") {
		t.Errorf("Logs(follow=false) args = %q, want no -f flag", data)
	}
}

func TestContainerHandle_Exec_PassesArgs(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	dir := fakeRuntimeBin(t, "docker", withArgsFile(argsFile))
	withFakePath(t, dir)

	h := &containerHandle{name: "foo", runtime: "docker"}
	if err := h.Exec(context.Background(), []string{"sh", "-c", "echo hi"}); err != nil {
		t.Fatalf("Exec: %v", err)
	}

	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("reading args file: %v", err)
	}
	args := string(data)
	for _, want := range []string{"exec", "-it", "foo", "sh", "-c", "echo hi"} {
		if !strings.Contains(args, want) {
			t.Errorf("Exec() args = %q, missing %q", args, want)
		}
	}
}

func TestContainerHandle_Wait_AlwaysNil(t *testing.T) {
	h := &containerHandle{name: "foo", runtime: "docker"}
	if err := h.Wait(context.Background()); err != nil {
		t.Errorf("Wait() = %v, want nil", err)
	}
}

func TestContainerHandle_PID_AlwaysZero(t *testing.T) {
	h := &containerHandle{name: "foo", runtime: "docker"}
	if pid := h.PID(); pid != 0 {
		t.Errorf("PID() = %d, want 0", pid)
	}
}
