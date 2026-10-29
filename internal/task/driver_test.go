package task

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dever-labs/devx/internal/config"
)

func TestShellFor_ExplicitOverrides(t *testing.T) {
	cases := []struct {
		override, wantShell, wantFlag string
	}{
		{"sh", "sh", "-c"},
		{"bash", "bash", "-c"},
		{"cmd", "cmd", "/C"},
		{"powershell", "powershell", "-Command"},
		{"pwsh", "pwsh", "-Command"},
		{"zsh", "zsh", "-c"}, // default fallthrough for any other custom shell
	}
	for _, c := range cases {
		shell, flag := shellFor(c.override)
		if shell != c.wantShell || flag != c.wantFlag {
			t.Errorf("shellFor(%q) = (%q, %q), want (%q, %q)", c.override, shell, flag, c.wantShell, c.wantFlag)
		}
	}
}

func TestShellFor_DefaultMatchesDocumentedGOOSBehavior(t *testing.T) {
	// docs/manifest.md documents sh -c on Linux/macOS, cmd /C on Windows
	// for the default (unset) shell -- see internal/task/driver.go.
	shell, flag := shellFor("")
	if runtime.GOOS == "windows" {
		if shell != "cmd" || flag != "/C" {
			t.Errorf("shellFor(\"\") on windows = (%q, %q), want (\"cmd\", \"/C\")", shell, flag)
		}
	} else {
		if shell != "sh" || flag != "-c" {
			t.Errorf("shellFor(\"\") on %s = (%q, %q), want (\"sh\", \"-c\")", runtime.GOOS, shell, flag)
		}
	}
}

func newEffectiveTask(name, run, dir string) *EffectiveTask {
	et := &EffectiveTask{}
	et.Name = name
	et.ResolvedRun = run
	et.ResolvedDir = dir
	et.ResolvedEnv = map[string]string{}
	return et
}

// newSleepTask is like newEffectiveTask but forces shell: sh, since "sleep"
// (unlike "echo"/"exit", which shellFor's platform-appropriate default
// shell handles natively everywhere) is a POSIX coreutils command with
// no cmd.exe equivalent — these tests need a real POSIX shell
// regardless of GOOS, which on Windows means requiring Git Bash (as
// devx's own docs note "sh" is settable via shell: for exactly this).
func newSleepTask(t *testing.T, name, run, dir string) *EffectiveTask {
	t.Helper()
	et := newEffectiveTask(name, run, dir)
	et.Shell = "sh"
	return et
}

func TestProcessDriver_StartWaitExit(t *testing.T) {
	d := NewProcessDriver()
	logPath := filepath.Join(t.TempDir(), "task.log")

	h, err := d.Start(context.Background(), newEffectiveTask("t", "echo hello-from-task", "."), logPath)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitErr := h.Wait(context.Background())

	data, readErr := os.ReadFile(logPath)
	if waitErr != nil {
		if readErr == nil {
			t.Fatalf("Wait: %v (captured output: %q)", waitErr, data)
		}
		t.Fatalf("Wait: %v (also failed to read log: %v)", waitErr, readErr)
	}
	if readErr != nil {
		t.Fatalf("read log: %v", readErr)
	}
	if !strings.Contains(string(data), "hello-from-task") {
		t.Errorf("expected log to contain task output, got %q", data)
	}
}

func TestProcessDriver_StatusTransitions(t *testing.T) {
	d := NewProcessDriver()
	logPath := filepath.Join(t.TempDir(), "task.log")

	h, err := d.Start(context.Background(), newSleepTask(t, "t", "sleep 0.3", "."), logPath)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	status, err := h.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status != "running" {
		t.Errorf("expected 'running' immediately after start, got %q", status)
	}

	if err := h.Wait(context.Background()); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	status, err = h.Status(context.Background())
	if err != nil {
		t.Fatalf("Status after exit: %v", err)
	}
	if status != "exited" {
		t.Errorf("expected 'exited' after a successful run, got %q", status)
	}
}

func TestProcessDriver_StatusCrashed(t *testing.T) {
	d := NewProcessDriver()
	logPath := filepath.Join(t.TempDir(), "task.log")

	h, err := d.Start(context.Background(), newEffectiveTask("t", "exit 1", "."), logPath)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Wait returns the process error (non-nil for a non-zero exit).
	_ = h.Wait(context.Background())

	status, err := h.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status != "crashed" {
		t.Errorf("expected 'crashed' for a non-zero exit, got %q", status)
	}
}

func TestProcessDriver_StopTerminatesProcess(t *testing.T) {
	d := NewProcessDriver()
	logPath := filepath.Join(t.TempDir(), "task.log")

	h, err := d.Start(context.Background(), newSleepTask(t, "t", "sleep 300", "."), logPath)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	pid := h.PID()
	if pid <= 0 {
		t.Fatalf("expected a positive PID, got %d", pid)
	}

	if err := h.Stop(context.Background(), 2*time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := h.Wait(context.Background()); err == nil {
		t.Log("process exited cleanly after SIGTERM (expected for 'sleep')")
	}
}

func TestProcessDriver_Exec(t *testing.T) {
	d := NewProcessDriver()
	logPath := filepath.Join(t.TempDir(), "task.log")
	h, err := d.Start(context.Background(), newEffectiveTask("t", "true", "."), logPath)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	_ = h.Wait(context.Background())

	if err := h.Exec(context.Background(), []string{"true"}); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if err := h.Exec(context.Background(), nil); err == nil {
		t.Fatal("expected an error when Exec is called with no args")
	}
}

func TestFollowReader_WaitsForAppendedData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "follow.log")
	if err := os.WriteFile(path, []byte("line1\n"), 0600); err != nil {
		t.Fatal(err)
	}

	rc, err := tailFile(path, true)
	if err != nil {
		t.Fatalf("tailFile: %v", err)
	}
	defer rc.Close()

	buf := make([]byte, 64)
	n, err := rc.Read(buf)
	if err != nil {
		t.Fatalf("initial Read: %v", err)
	}
	if string(buf[:n]) != "line1\n" {
		t.Fatalf("expected 'line1\\n', got %q", buf[:n])
	}

	// Append more data in the background, after the reader has already
	// hit EOF once — this is exactly the busy-spin scenario the fixed
	// followReader must handle without pegging a CPU core.
	go func() {
		time.Sleep(100 * time.Millisecond)
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = f.WriteString("line2\n")
	}()

	n, err = rc.Read(buf)
	if err != nil {
		t.Fatalf("Read after append: %v", err)
	}
	if string(buf[:n]) != "line2\n" {
		t.Fatalf("expected 'line2\\n', got %q", buf[:n])
	}
}

func TestTailFile_NoFollow_ReturnsPlainFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain.log")
	if err := os.WriteFile(path, []byte("hello\n"), 0600); err != nil {
		t.Fatal(err)
	}
	rc, err := tailFile(path, false)
	if err != nil {
		t.Fatalf("tailFile: %v", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "hello\n" {
		t.Errorf("got %q, want %q", data, "hello\n")
	}
}

func TestCheckReady_Cmd(t *testing.T) {
	// "exit 0"/"exit 1" (rather than the POSIX-only "true"/"false") work
	// identically under both shellFor defaults (sh -c, cmd /C).
	if err := CheckReady(context.Background(), &config.ReadyCheck{Cmd: "exit 0"}, "."); err != nil {
		t.Errorf("expected a zero-exit command to succeed, got %v", err)
	}
	if err := CheckReady(context.Background(), &config.ReadyCheck{Cmd: "exit 1"}, "."); err == nil {
		t.Error("expected a non-zero-exit command to fail readiness")
	}
}

func TestCheckReady_NilReturnsNoError(t *testing.T) {
	if err := CheckReady(context.Background(), nil, "."); err != nil {
		t.Errorf("expected nil ready: to always be ready, got %v", err)
	}
}

func TestCheckReady_HTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := CheckReady(context.Background(), &config.ReadyCheck{HTTP: srv.URL}, "."); err != nil {
		t.Errorf("expected reachable HTTP server to be ready, got %v", err)
	}
}

func TestCheckReady_HTTP_UnreachableFallsBackToTCP(t *testing.T) {
	// A closed listener's address is guaranteed refused.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	err = CheckReady(context.Background(), &config.ReadyCheck{HTTP: "http://" + addr}, ".")
	if err == nil {
		t.Error("expected an error when both HTTP and its TCP fallback are unreachable")
	}
}

func TestCheckReady_TCP(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	if err := CheckReady(context.Background(), &config.ReadyCheck{TCP: l.Addr().String()}, "."); err != nil {
		t.Errorf("expected listening TCP addr to be ready, got %v", err)
	}
}

func TestCheckReady_TCP_Unreachable(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	if err := CheckReady(context.Background(), &config.ReadyCheck{TCP: addr}, "."); err == nil {
		t.Error("expected an error for an unreachable TCP addr")
	}
}

func TestWaitReady_SucceedsOnceReady(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := WaitReady(ctx, &config.ReadyCheck{TCP: l.Addr().String()}, "."); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
}

func TestWaitReady_TimesOut(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := WaitReady(ctx, &config.ReadyCheck{TCP: addr}, "."); err == nil {
		t.Fatal("expected WaitReady to time out against an unreachable address")
	}
}

func TestWaitReady_NilIsImmediatelyReady(t *testing.T) {
	if err := WaitReady(context.Background(), nil, "."); err != nil {
		t.Errorf("expected nil ready: to be immediately ready, got %v", err)
	}
}
