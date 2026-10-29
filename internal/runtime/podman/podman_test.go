package podman

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dever-labs/devx/internal/runtime"
)

func TestNew_ReturnsPodmanBinary(t *testing.T) {
	r := New()
	if r.Binary != "podman" {
		t.Errorf("New().Binary = %q, want %q", r.Binary, "podman")
	}
}

func TestName_ReturnsPodman(t *testing.T) {
	if got := New().Name(); got != "podman" {
		t.Errorf("Name() = %q, want %q", got, "podman")
	}
}

func TestDetect_RealPodmanNotInstalledReturnsFalseNoError(t *testing.T) {
	// podman is not installed in this sandbox; New() should still fail
	// gracefully rather than returning an error.
	if _, err := os.Stat("/usr/bin/podman"); err == nil {
		t.Skip("podman unexpectedly installed in this environment")
	}
	ok, err := New().Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() should swallow exec errors, got: %v", err)
	}
	if ok {
		t.Error("expected Detect() to report false when podman is not installed")
	}
}

func TestDetect_MissingBinaryReturnsFalseNoError(t *testing.T) {
	r := &Runtime{Binary: "devx-definitely-not-a-real-binary"}
	ok, err := r.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() should swallow exec errors, got: %v", err)
	}
	if ok {
		t.Error("expected Detect() to report false for a missing binary")
	}
}

func TestUp_BuildsExpectedArgs(t *testing.T) {
	bin := writeFakeBinary(t)
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("FAKE_ARGS_FILE", argsFile)

	r := &Runtime{Binary: bin}
	err := r.Up(context.Background(), "docker-compose.yml", "myproj", runtime.UpOptions{Build: true, Pull: true})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}

	args := readArgsFile(t, argsFile)
	for _, want := range []string{"compose", "-f", "docker-compose.yml", "-p", "myproj", "up", "-d", "--build", "--pull", "always"} {
		if !containsArg(args, want) {
			t.Errorf("Up() args = %v, missing %q", args, want)
		}
	}
}

func TestUp_WithoutOptionsOmitsFlags(t *testing.T) {
	bin := writeFakeBinary(t)
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("FAKE_ARGS_FILE", argsFile)

	r := &Runtime{Binary: bin}
	if err := r.Up(context.Background(), "c.yml", "p", runtime.UpOptions{}); err != nil {
		t.Fatalf("Up: %v", err)
	}
	args := readArgsFile(t, argsFile)
	if containsArg(args, "--build") || containsArg(args, "--pull") {
		t.Errorf("Up() args = %v, expected no --build/--pull flags", args)
	}
}

func TestDown_RemoveVolumesFlag(t *testing.T) {
	bin := writeFakeBinary(t)
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("FAKE_ARGS_FILE", argsFile)

	r := &Runtime{Binary: bin}
	if err := r.Down(context.Background(), "c.yml", "p", true); err != nil {
		t.Fatalf("Down: %v", err)
	}
	args := readArgsFile(t, argsFile)
	if !containsArg(args, "--volumes") {
		t.Errorf("Down(removeVolumes=true) args = %v, want --volumes", args)
	}
}

func TestDown_WithoutRemoveVolumes(t *testing.T) {
	bin := writeFakeBinary(t)
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("FAKE_ARGS_FILE", argsFile)

	r := &Runtime{Binary: bin}
	if err := r.Down(context.Background(), "c.yml", "p", false); err != nil {
		t.Fatalf("Down: %v", err)
	}
	args := readArgsFile(t, argsFile)
	if containsArg(args, "--volumes") {
		t.Errorf("Down(removeVolumes=false) args = %v, want no --volumes", args)
	}
}

func TestExec_ReturnsNonZeroExitCodeWithoutError(t *testing.T) {
	bin := writeFakeBinary(t)
	t.Setenv("FAKE_EXIT_CODE", "7")

	r := &Runtime{Binary: bin}
	code, err := r.Exec(context.Background(), "c.yml", "p", "api", []string{"false"})
	if err != nil {
		t.Fatalf("Exec() should not return an error for a nonzero exit code, got: %v", err)
	}
	if code != 7 {
		t.Errorf("Exec() exit code = %d, want 7", code)
	}
}

func TestExec_ReturnsZeroOnSuccess(t *testing.T) {
	bin := writeFakeBinary(t)
	r := &Runtime{Binary: bin}
	code, err := r.Exec(context.Background(), "c.yml", "p", "api", []string{"true"})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if code != 0 {
		t.Errorf("Exec() exit code = %d, want 0", code)
	}
}

func TestExec_MissingBinaryReturnsError(t *testing.T) {
	r := &Runtime{Binary: "devx-definitely-not-a-real-binary"}
	code, err := r.Exec(context.Background(), "c.yml", "p", "api", []string{"true"})
	if err == nil {
		t.Fatal("expected an error for a missing binary")
	}
	if code != 1 {
		t.Errorf("Exec() exit code = %d, want 1 for an exec failure", code)
	}
}

func TestLogs_StreamsOutputAndCloses(t *testing.T) {
	bin := writeFakeBinary(t)
	stdoutFile := filepath.Join(t.TempDir(), "stdout.txt")
	if err := os.WriteFile(stdoutFile, []byte("line one\nline two\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_STDOUT_FILE", stdoutFile)

	r := &Runtime{Binary: bin}
	rc, err := r.Logs(context.Background(), "c.yml", "p", runtime.LogsOptions{Follow: true, Since: "10m", Service: "api"})
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	defer rc.Close()

	buf := make([]byte, 1024)
	deadline := time.Now().Add(3 * time.Second)
	var total []byte
	for time.Now().Before(deadline) {
		n, err := rc.Read(buf)
		total = append(total, buf[:n]...)
		if err != nil {
			break
		}
		if len(total) > 0 {
			break
		}
	}
	if len(total) == 0 {
		t.Fatal("expected to read some log output")
	}
}

func TestStatus_ParsesJSONArrayEntries(t *testing.T) {
	bin := writeFakeBinary(t)
	stdoutFile := filepath.Join(t.TempDir(), "stdout.txt")
	payload := `[{"Service":"api","State":"running","Health":"healthy","Publishers":[{"URL":"0.0.0.0","TargetPort":8080,"PublishedPort":8080,"Protocol":"tcp"}]}]`
	if err := os.WriteFile(stdoutFile, []byte(payload), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_STDOUT_FILE", stdoutFile)

	r := &Runtime{Binary: bin}
	statuses, err := r.Status(context.Background(), "c.yml", "p")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %+v", statuses)
	}
	s := statuses[0]
	if s.Name != "api" || s.State != "running" || s.Health != "healthy" {
		t.Errorf("status = %+v, want api/running/healthy", s)
	}
	if len(s.Publishers) != 1 || s.Publishers[0].PublishedPort != 8080 {
		t.Errorf("status.Publishers = %+v, want one publisher on port 8080", s.Publishers)
	}
}

func TestStatus_ParsesNDJSONEntries(t *testing.T) {
	bin := writeFakeBinary(t)
	stdoutFile := filepath.Join(t.TempDir(), "stdout.txt")
	payload := "{\"Service\":\"api\",\"State\":\"running\"}\n{\"Service\":\"db\",\"State\":\"exited\"}\n"
	if err := os.WriteFile(stdoutFile, []byte(payload), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_STDOUT_FILE", stdoutFile)

	r := &Runtime{Binary: bin}
	statuses, err := r.Status(context.Background(), "c.yml", "p")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %+v", statuses)
	}
	if statuses[0].Name != "api" || statuses[1].Name != "db" {
		t.Errorf("statuses = %+v, want api then db", statuses)
	}
}

func TestStatus_InvalidNDJSONLineErrors(t *testing.T) {
	bin := writeFakeBinary(t)
	stdoutFile := filepath.Join(t.TempDir(), "stdout.txt")
	if err := os.WriteFile(stdoutFile, []byte("not json at all"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_STDOUT_FILE", stdoutFile)

	r := &Runtime{Binary: bin}
	if _, err := r.Status(context.Background(), "c.yml", "p"); err == nil {
		t.Fatal("expected an error for invalid status output")
	}
}

func TestStatus_CommandFailureReturnsError(t *testing.T) {
	bin := writeFakeBinary(t)
	t.Setenv("FAKE_EXIT_CODE", "1")

	r := &Runtime{Binary: bin}
	if _, err := r.Status(context.Background(), "c.yml", "p"); err == nil {
		t.Fatal("expected an error when the compose command fails")
	}
}

func TestResolveImageDigest_UsesCachedDigestWithoutPulling(t *testing.T) {
	bin := writeFakeBinary(t)
	stdoutFile := filepath.Join(t.TempDir(), "stdout.txt")
	if err := os.WriteFile(stdoutFile, []byte("myimage@sha256:abc123\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_STDOUT_FILE", stdoutFile)

	r := &Runtime{Binary: bin}
	digest, err := r.ResolveImageDigest(context.Background(), "myimage:latest")
	if err != nil {
		t.Fatalf("ResolveImageDigest: %v", err)
	}
	if digest != "sha256:abc123" {
		t.Errorf("digest = %q, want sha256:abc123", digest)
	}
}

func TestResolveRepoDigest_NoDigestFoundErrors(t *testing.T) {
	bin := writeFakeBinary(t)
	stdoutFile := filepath.Join(t.TempDir(), "stdout.txt")
	if err := os.WriteFile(stdoutFile, []byte("\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_STDOUT_FILE", stdoutFile)

	_, err := resolveRepoDigest(context.Background(), bin, "myimage")
	if err == nil {
		t.Fatal("expected an error when no digest line is found")
	}
}

func TestResolveRepoDigest_CommandFailureErrors(t *testing.T) {
	bin := writeFakeBinary(t)
	t.Setenv("FAKE_EXIT_CODE", "1")
	if _, err := resolveRepoDigest(context.Background(), bin, "myimage"); err == nil {
		t.Fatal("expected an error when the inspect command fails")
	}
}

func TestCommandReader_ReadAndClose(t *testing.T) {
	bin := writeFakeBinary(t)
	stdoutFile := filepath.Join(t.TempDir(), "stdout.txt")
	if err := os.WriteFile(stdoutFile, []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_STDOUT_FILE", stdoutFile)

	r := &Runtime{Binary: bin}
	rc, err := r.Logs(context.Background(), "c.yml", "p", runtime.LogsOptions{})
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Errorf("Close() returned an unexpected error: %v", err)
	}
}
