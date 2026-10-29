package task

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dever-labs/devx/internal/config"
)

// Handle represents a running or completed task instance, regardless of
// whether it's backed by a live OS process (processDriver) or an
// unrelated external container addressed purely by name (commandDriver
// for container-shaped run:).
type Handle interface {
	// Wait blocks until a one-shot task's recipe exits, returning its
	// exit code. For a detached task, Wait returns once readiness is
	// confirmed (or times out), not at process exit.
	Wait(ctx context.Context) error
	// Status reports the current state: running, ready, exited, crashed.
	Status(ctx context.Context) (string, error)
	// Stop gracefully stops a detached task (SIGTERM/docker stop, grace
	// period, then force).
	Stop(ctx context.Context, timeout time.Duration) error
	// Logs streams the task's captured output.
	Logs(ctx context.Context, follow bool) (io.ReadCloser, error)
	// Exec runs an additional command inside the task's context.
	Exec(ctx context.Context, args []string) error
	// PID returns the OS process group ID devx can signal to stop this
	// task from a later CLI invocation (processHandle only). Returns 0
	// for container-shaped tasks, which are instead stopped by name.
	PID() int
}

// Driver launches an EffectiveTask and returns a Handle to it.
type Driver interface {
	Start(ctx context.Context, et *EffectiveTask, logPath string) (Handle, error)
}

// processDriver launches run: as a real OS subprocess devx holds a live
// reference to — used whenever run: isn't recognized as a container
// launch (IsContainer false). Built from scratch: no pre-existing
// supervisor in this codebase to wrap.
type processDriver struct{}

func NewProcessDriver() Driver { return &processDriver{} }

type processHandle struct {
	cmd     *exec.Cmd
	logFile *os.File
	logPath string
	// exited is closed exactly once, when cmd.Wait() returns — this lets
	// Wait and Status be called any number of times (in any order) and
	// still observe a consistent terminal result, unlike a single-value
	// channel that only the first reader can drain.
	exited  chan struct{}
	exitErr error // valid only once exited is closed
}

func (d *processDriver) Start(ctx context.Context, et *EffectiveTask, logPath string) (Handle, error) {
	shellBin, shellArg := shellFor(et.Shell)
	cmd := exec.Command(shellBin, shellArg, et.ResolvedRun)
	cmd.Dir = et.ResolvedDir
	cmd.Env = append(os.Environ(), envSlice(et.ResolvedEnv)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	setPgid(cmd.SysProcAttr)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	if et.Interactive {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("start task %q: %w", et.Name, err)
	}

	h := &processHandle{cmd: cmd, logFile: logFile, logPath: logPath, exited: make(chan struct{})}
	go func() {
		h.exitErr = cmd.Wait()
		// Close the log file before signaling exited, so that any Wait
		// or Stop caller that observes process exit can also rely on the
		// log file handle having been released (e.g. so a subsequent
		// os.Remove/RemoveAll of the log's directory doesn't race with
		// this goroutine on platforms like Windows that forbid deleting
		// open files).
		logFile.Close()
		close(h.exited)
	}()
	return h, nil
}

func shellFor(override string) (string, string) {
	switch override {
	case "":
		// Match docs/manifest.md's documented default: sh -c on
		// Linux/macOS, cmd /C on Windows (where a POSIX "sh" isn't
		// guaranteed to be on PATH at all, and isn't reliable even when
		// it is -- see issue #39).
		if runtime.GOOS == "windows" {
			return "cmd", "/C"
		}
		return "sh", "-c"
	case "sh":
		return "sh", "-c"
	case "bash":
		return "bash", "-c"
	case "cmd":
		return "cmd", "/C"
	case "powershell", "pwsh":
		return override, "-Command"
	default:
		return override, "-c"
	}
}

func envSlice(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

func (h *processHandle) Wait(ctx context.Context) error {
	select {
	case <-h.exited:
		return h.exitErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *processHandle) Status(ctx context.Context) (string, error) {
	select {
	case <-h.exited:
		if h.exitErr != nil {
			return "crashed", nil
		}
		return "exited", nil
	default:
		return "running", nil
	}
}

func (h *processHandle) Stop(ctx context.Context, timeout time.Duration) error {
	if h.cmd.Process == nil {
		return nil
	}
	pid := h.cmd.Process.Pid
	stopProcessTree(pid)

	select {
	case <-h.exited:
		return nil
	case <-time.After(timeout):
		killProcessTree(pid)
		return nil
	}
}

func (h *processHandle) Logs(ctx context.Context, follow bool) (io.ReadCloser, error) {
	return tailFile(h.logPath, follow)
}

func (h *processHandle) Exec(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("exec requires a command")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (h *processHandle) PID() int {
	if h.cmd.Process == nil {
		return 0
	}
	return h.cmd.Process.Pid
}

// containerDriver addresses a container-shaped run: purely by name
// through the Docker CLI, per "Graceful stop, driver-aware": once
// run: has done `docker run -d`, the launching process has already
// exited, so there's nothing to hold a live reference to — every
// subsequent operation (Stop/Status/Logs/Exec) shells out to <name>.
type containerDriver struct{}

func NewContainerDriver() Driver { return &containerDriver{} }

type containerHandle struct {
	name    string
	runtime string
}

func (d *containerDriver) Start(ctx context.Context, et *EffectiveTask, logPath string) (Handle, error) {
	shellBin, shellArg := shellFor(et.Shell)
	cmd := exec.CommandContext(ctx, shellBin, shellArg, et.ResolvedRun)
	cmd.Dir = et.ResolvedDir
	cmd.Env = append(os.Environ(), envSlice(et.ResolvedEnv)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("start task %q: %w: %s", et.Name, err, string(out))
	}
	if et.ContainerName == "" {
		return nil, fmt.Errorf("task %q: run: looks like a container launch but no name could be parsed or overridden via containerName: — devx can't Stop()/exec into it later", et.Name)
	}
	runtime := "docker"
	if strings.Contains(et.ResolvedRun, "podman") {
		runtime = "podman"
	}
	return &containerHandle{name: et.ContainerName, runtime: runtime}, nil
}

func (h *containerHandle) Wait(ctx context.Context) error { return nil }

func (h *containerHandle) Status(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, h.runtime, "inspect", "--format", "{{.State.Status}}", h.name).CombinedOutput()
	if err != nil {
		return "exited", nil
	}
	status := strings.TrimSpace(string(out))
	if status == "running" {
		return "running", nil
	}
	return status, nil
}

func (h *containerHandle) Stop(ctx context.Context, timeout time.Duration) error {
	secs := strconv.Itoa(int(timeout.Seconds()))
	cmd := exec.CommandContext(ctx, h.runtime, "stop", "-t", secs, h.name)
	return cmd.Run()
}

func (h *containerHandle) Logs(ctx context.Context, follow bool) (io.ReadCloser, error) {
	args := []string{"logs"}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, h.name)
	cmd := exec.CommandContext(ctx, h.runtime, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return stdout, nil
}

func (h *containerHandle) Exec(ctx context.Context, args []string) error {
	full := append([]string{"exec", "-it", h.name}, args...)
	cmd := exec.CommandContext(ctx, h.runtime, full...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (h *containerHandle) PID() int { return 0 }

// tailFile returns a ReadCloser over path's existing contents, and
// (when follow is true) subsequent appends, polling for new data.
func tailFile(path string, follow bool) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if !follow {
		return f, nil
	}
	return &followReader{f: f}, nil
}

type followReader struct {
	f *os.File
}

func (r *followReader) Read(p []byte) (int, error) {
	for {
		n, err := r.f.Read(p)
		if n > 0 {
			return n, nil
		}
		if err != nil && err != io.EOF {
			return 0, err
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func (r *followReader) Close() error { return r.f.Close() }

// CheckReady performs the documented readiness check: explicit cmd: if
// set, else HTTP GET with TCP fallback, else immediately ready.
func CheckReady(ctx context.Context, ready *config.ReadyCheck, dir string) error {
	if ready == nil {
		return nil
	}
	if ready.Cmd != "" {
		shellBin, shellArg := shellFor("")
		cmd := exec.CommandContext(ctx, shellBin, shellArg, ready.Cmd)
		cmd.Dir = dir
		return cmd.Run()
	}
	if ready.HTTP != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ready.HTTP, nil)
		if err == nil {
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				resp.Body.Close()
				return nil
			}
		}
		if ready.TCP != "" {
			return tcpCheck(ctx, ready.TCP)
		}
		return tcpCheckFromHTTP(ctx, ready.HTTP)
	}
	if ready.TCP != "" {
		return tcpCheck(ctx, ready.TCP)
	}
	return nil
}

func tcpCheck(ctx context.Context, addr string) error {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

func tcpCheckFromHTTP(ctx context.Context, httpURL string) error {
	addr := strings.TrimPrefix(httpURL, "http://")
	addr = strings.TrimPrefix(addr, "https://")
	if i := strings.Index(addr, "/"); i >= 0 {
		addr = addr[:i]
	}
	return tcpCheck(ctx, addr)
}

// WaitReady polls CheckReady until it succeeds or ctx times out.
func WaitReady(ctx context.Context, ready *config.ReadyCheck, dir string) error {
	if ready == nil {
		return nil
	}
	interval := 300 * time.Millisecond
	for {
		if err := CheckReady(ctx, ready, dir); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for readiness")
		case <-time.After(interval):
		}
	}
}
