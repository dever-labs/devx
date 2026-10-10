// Package process supervises long-running host processes (dev servers such as
// `yarn dev`) for devx. Processes are started detached so `devx up` can return;
// their PIDs and log files are tracked under the .devx directory.
package process

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Spec describes a process to run.
type Spec struct {
	Name    string
	Command []string // argv; mutually exclusive with Run
	Run     string   // shell command line
	Workdir string
	Env     map[string]string
}

// Record is the persisted state for one supervised process.
type Record struct {
	Name      string    `json:"name"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"startedAt"`
	Cmd       string    `json:"cmd"`
	Workdir   string    `json:"workdir,omitempty"`
	LogFile   string    `json:"logFile"`
}

// Supervisor manages processes whose state lives in dir (normally ".devx").
type Supervisor struct {
	dir string
	mu  sync.Mutex
}

func New(dir string) *Supervisor { return &Supervisor{dir: dir} }

func (s *Supervisor) stateFile() string { return filepath.Join(s.dir, "processes.json") }
func (s *Supervisor) logDir() string    { return filepath.Join(s.dir, "logs") }

// LogPath returns the log file path for a service.
func (s *Supervisor) LogPath(name string) string {
	return filepath.Join(s.logDir(), name+".log")
}

func describe(sp Spec) string {
	if sp.Run != "" {
		return sp.Run
	}
	return strings.Join(sp.Command, " ")
}

func buildCmd(sp Spec) (*exec.Cmd, error) {
	var cmd *exec.Cmd
	switch {
	case sp.Run != "":
		if goruntime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/C", sp.Run)
		} else {
			cmd = exec.Command("sh", "-c", sp.Run)
		}
	case len(sp.Command) > 0:
		cmd = exec.Command(sp.Command[0], sp.Command[1:]...)
	default:
		return nil, fmt.Errorf("process %q has no command", sp.Name)
	}
	cmd.Dir = sp.Workdir
	cmd.Env = os.Environ()
	for _, k := range sortedKeys(sp.Env) {
		cmd.Env = append(cmd.Env, k+"="+sp.Env[k])
	}
	return cmd, nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (s *Supervisor) load() (map[string]Record, error) {
	data, err := os.ReadFile(s.stateFile())
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	recs := map[string]Record{}
	if err := json.Unmarshal(data, &recs); err != nil {
		return map[string]Record{}, nil
	}
	return recs, nil
}

func (s *Supervisor) save(recs map[string]Record) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(recs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.stateFile(), data, 0o600)
}

// Get returns the record for name and whether its process is still alive.
func (s *Supervisor) Get(name string) (Record, bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	recs, _ := s.load()
	r, ok := recs[name]
	if !ok {
		return Record{}, false, false
	}
	return r, true, alive(r.PID)
}

// List returns all recorded processes sorted by name.
func (s *Supervisor) List() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	recs, _ := s.load()
	out := make([]Record, 0, len(recs))
	for _, r := range recs {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Alive reports whether the recorded process for name is running.
func (s *Supervisor) Alive(name string) bool {
	_, ok, running := s.Get(name)
	return ok && running
}

// Start launches sp detached. If an identical process is already running it is
// left alone and started=false is returned.
func (s *Supervisor) Start(sp Spec) (rec Record, started bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	recs, err := s.load()
	if err != nil {
		return Record{}, false, err
	}
	cmdline := describe(sp)
	if r, ok := recs[sp.Name]; ok && alive(r.PID) {
		if r.Cmd == cmdline {
			return r, false, nil
		}
		return Record{}, false, fmt.Errorf("%s is already running with a different command (pid %d); run 'devx down' first", sp.Name, r.PID)
	}

	if err := os.MkdirAll(s.logDir(), 0o755); err != nil {
		return Record{}, false, err
	}
	logPath := s.LogPath(sp.Name)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return Record{}, false, err
	}
	defer logFile.Close()

	cmd, err := buildCmd(sp)
	if err != nil {
		return Record{}, false, err
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return Record{}, false, fmt.Errorf("starting %s: %w", sp.Name, err)
	}
	pid := cmd.Process.Pid
	// The child is intentionally not waited on; release OS resources.
	_ = cmd.Process.Release()

	rec = Record{Name: sp.Name, PID: pid, StartedAt: time.Now().UTC(), Cmd: cmdline, Workdir: sp.Workdir, LogFile: logPath}
	recs[sp.Name] = rec
	if err := s.save(recs); err != nil {
		return rec, true, err
	}
	return rec, true, nil
}

// Stop terminates the process tree for name (graceful first, then forced) and
// forgets it. Stopping an unknown or already-dead process is not an error.
func (s *Supervisor) Stop(name string, grace time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	recs, err := s.load()
	if err != nil {
		return err
	}
	r, ok := recs[name]
	if !ok {
		return nil
	}
	if alive(r.PID) {
		terminate(r.PID, grace)
	}
	delete(recs, name)
	return s.save(recs)
}

// StopAll stops every recorded process.
func (s *Supervisor) StopAll(grace time.Duration) error {
	var errs []error
	recs := s.List()
	for i := len(recs) - 1; i >= 0; i-- {
		if err := s.Stop(recs[i].Name, grace); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// RunForeground runs sp attached to the current terminal (stdio inherited so
// debuggers and interactive tools work) and returns its exit code.
func RunForeground(ctx context.Context, sp Spec) (int, error) {
	cmd, err := buildCmd(sp)
	if err != nil {
		return 1, err
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return 1, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return exitCode(err)
	case <-ctx.Done():
		terminate(cmd.Process.Pid, 5*time.Second)
		<-done
		return 130, ctx.Err()
	}
}

func exitCode(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	return 1, err
}

// Exec runs argv in the service's workdir/env with inherited stdio.
func Exec(ctx context.Context, sp Spec, argv []string) (int, error) {
	return RunForeground(ctx, Spec{Name: sp.Name, Command: argv, Workdir: sp.Workdir, Env: sp.Env})
}

// Tail returns up to n trailing lines of the service log.
func (s *Supervisor) Tail(name string, n int) []string {
	f, err := os.Open(s.LogPath(name))
	if err != nil {
		return nil
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	return lines
}

// Follow streams the log of name to emit, one line at a time. When follow is
// false it returns after reaching the end of the current file.
func (s *Supervisor) Follow(ctx context.Context, name string, follow bool, emit func(line string)) error {
	f, err := os.Open(s.LogPath(name))
	if err != nil {
		return fmt.Errorf("no logs for %s: %w", name, err)
	}
	defer f.Close()
	r := bufio.NewReader(f)
	var partial string
	for {
		chunk, err := r.ReadString('\n')
		partial += chunk
		if err == nil {
			emit(strings.TrimRight(partial, "\r\n"))
			partial = ""
			continue
		}
		if !errors.Is(err, io.EOF) {
			return err
		}
		if !follow {
			if partial != "" {
				emit(partial)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(200 * time.Millisecond):
		}
	}
}
