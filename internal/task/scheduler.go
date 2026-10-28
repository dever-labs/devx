package task

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/dever-labs/devx/internal/config"
	"github.com/dever-labs/devx/internal/graph"
)

// Options configures one scheduler run (devx up/run/down).
type Options struct {
	DryRun    bool
	KeepGoing bool
	Group     string
	CLIVars   map[string]string
	LogDir    string
	StateFile string
	Timeout   time.Duration
	Writer    func(task string, line string)
}

// Scheduler walks a task graph in dependency order, running independent
// branches in parallel, and tracks started Handles for status/logs/exec/
// down to reconstruct across separate CLI invocations.
type Scheduler struct {
	Manifest *config.Manifest
	Graph    *graph.Graph
	Opts     Options
	State    *StateStore

	mu      sync.Mutex
	handles map[string]Handle
	dotEnv  map[string]string
}

func NewScheduler(m *config.Manifest, opts Options) (*Scheduler, error) {
	g, err := graph.Build(m.Tasks)
	if err != nil {
		return nil, err
	}
	dotEnv, err := LoadDotEnv(m.Dir())
	if err != nil {
		return nil, fmt.Errorf("load .env: %w", err)
	}
	state, err := LoadState(opts.StateFile)
	if err != nil {
		return nil, err
	}
	if opts.LogDir != "" {
		if err := os.MkdirAll(opts.LogDir, 0755); err != nil {
			return nil, err
		}
	}
	return &Scheduler{
		Manifest: m,
		Graph:    g,
		Opts:     opts,
		State:    state,
		handles:  map[string]Handle{},
		dotEnv:   dotEnv,
	}, nil
}

func (s *Scheduler) resolverFor(t config.Task) *Resolver {
	return NewResolver(s.Opts.CLIVars, s.dotEnv, t.ScopeVars)
}

// ResolveOne substitutes+infers a single task by name.
func (s *Scheduler) ResolveOne(name string) (*EffectiveTask, error) {
	t, ok := s.Manifest.Tasks[name]
	if !ok {
		return nil, fmt.Errorf("unknown task %q", name)
	}
	return Resolve(t, s.resolverFor(t))
}

// ResolveAll resolves every task in the manifest, for --dry-run/tasks/validate.
func (s *Scheduler) ResolveAll() (map[string]*EffectiveTask, error) {
	out := map[string]*EffectiveTask{}
	for name, t := range s.Manifest.Tasks {
		et, err := Resolve(t, s.resolverFor(t))
		if err != nil {
			return nil, err
		}
		out[name] = et
	}
	return out, nil
}

func driverFor(et *EffectiveTask) Driver {
	if et.IsContainer {
		return NewContainerDriver()
	}
	return NewProcessDriver()
}

// Up runs every task in the closure of roots (or the whole graph if roots
// is empty) in dependency order, parallelizing independent branches.
// Idempotent: a detached task already running per the state file is
// skipped and reported, not restarted.
func (s *Scheduler) Up(ctx context.Context, roots []string) error {
	targets, order, err := s.selection(roots)
	if err != nil {
		return err
	}

	levels := levelize(s.Graph, order, targets)
	for _, level := range levels {
		if err := s.runLevel(ctx, level, true); err != nil && !s.Opts.KeepGoing {
			return err
		}
	}
	return s.State.Save(s.Opts.StateFile)
}

// Run executes the named tasks directly (deps first), attached: detached
// tasks run in the foreground instead of backgrounding.
func (s *Scheduler) Run(ctx context.Context, names []string) error {
	targets, order, err := s.selection(names)
	if err != nil {
		return err
	}
	levels := levelize(s.Graph, order, targets)
	for i, level := range levels {
		last := i == len(levels)-1
		foreground := last
		if err := s.runLevel(ctx, level, !foreground); err != nil && !s.Opts.KeepGoing {
			return err
		}
	}
	return s.State.Save(s.Opts.StateFile)
}

func (s *Scheduler) selection(roots []string) (map[string]bool, []string, error) {
	order, err := graph.TopoSort(s.Graph)
	if err != nil {
		return nil, nil, err
	}
	targets := map[string]bool{}
	for name := range s.Manifest.Tasks {
		targets[name] = true
	}
	if len(roots) > 0 {
		targets, err = graph.Closure(s.Graph, roots)
		if err != nil {
			return nil, nil, err
		}
	}
	if s.Opts.Group != "" {
		filtered := map[string]bool{}
		for name := range targets {
			if s.Manifest.Tasks[name].Group == s.Opts.Group {
				closure, err := graph.Closure(s.Graph, []string{name})
				if err != nil {
					return nil, nil, err
				}
				for n := range closure {
					filtered[n] = true
				}
			}
		}
		targets = filtered
	}
	return targets, order, nil
}

// levelize groups order into dependency levels so independent branches
// within a level can run in parallel while respecting deps across levels.
func levelize(g *graph.Graph, order []string, targets map[string]bool) [][]string {
	depth := map[string]int{}
	for _, name := range order {
		if !targets[name] {
			continue
		}
		max := -1
		for _, dep := range g.Nodes[name].DependsOn {
			if !targets[dep] {
				continue
			}
			if depth[dep] > max {
				max = depth[dep]
			}
		}
		depth[name] = max + 1
	}
	maxDepth := -1
	for _, d := range depth {
		if d > maxDepth {
			maxDepth = d
		}
	}
	levels := make([][]string, maxDepth+1)
	for name, d := range depth {
		levels[d] = append(levels[d], name)
	}
	for i := range levels {
		sort.Strings(levels[i])
	}
	return levels
}

func (s *Scheduler) runLevel(ctx context.Context, names []string, backgroundDetached bool) error {
	var wg sync.WaitGroup
	errs := make([]error, len(names))
	for i, name := range names {
		i, name := i, name
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = s.runOne(ctx, name, backgroundDetached)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Scheduler) runOne(ctx context.Context, name string, backgroundDetached bool) error {
	t := s.Manifest.Tasks[name]
	et, err := Resolve(t, s.resolverFor(t))
	if err != nil {
		return err
	}

	if !osMatches(et.OS) {
		s.emit(name, "skipped (os: doesn't match current platform)")
		return nil
	}

	if upToDate(et) {
		s.emit(name, "up to date, skipping")
		return nil
	}

	if existing, ok := s.State.Lookup(name); ok && et.Detached {
		if alive := s.probeExisting(ctx, et, existing); alive {
			s.emit(name, "already running, skipping")
			return nil
		}
	}

	if s.Opts.DryRun {
		s.emit(name, "[dry-run] "+et.ResolvedRun)
		return nil
	}

	logPath := filepath.Join(s.Opts.LogDir, sanitize(name)+".log")
	driver := driverFor(et)
	timeoutCtx := ctx
	var cancel context.CancelFunc
	if to := taskTimeout(et, s.Opts.Timeout); to > 0 {
		timeoutCtx, cancel = context.WithTimeout(ctx, to)
		defer cancel()
	}

	handle, err := driver.Start(timeoutCtx, et, logPath)
	if err != nil {
		return fmt.Errorf("task %q: %w", name, err)
	}

	s.mu.Lock()
	s.handles[name] = handle
	s.mu.Unlock()

	if et.Detached {
		if err := WaitReady(timeoutCtx, et.ResolvedReady, et.ResolvedDir); err != nil {
			return fmt.Errorf("task %q: %w", name, err)
		}
		s.State.Record(name, StateEntry{
			ContainerName: et.ContainerName,
			IsContainer:   et.IsContainer,
			LogPath:       logPath,
			StartedAt:     time.Now(),
			PID:           handle.PID(),
		})
		s.emit(name, "ready")
		if !backgroundDetached {
			return handle.Wait(ctx)
		}
		return nil
	}

	s.emit(name, "running")
	err = handle.Wait(timeoutCtx)
	if err != nil {
		return fmt.Errorf("task %q failed: %w", name, err)
	}
	s.emit(name, "done")
	return nil
}

func (s *Scheduler) probeExisting(ctx context.Context, et *EffectiveTask, existing StateEntry) bool {
	if existing.IsContainer {
		h := &containerHandle{name: existing.ContainerName, runtime: "docker"}
		status, err := h.Status(ctx)
		return err == nil && status == "running"
	}
	// Plain processes aren't re-attachable across invocations without a
	// held OS handle, but the stored PID still lets us rule out the
	// common case where the process has already died (e.g. the host
	// rebooted, or a prior `devx down` killed it directly by PID).
	if existing.PID > 0 && !processAlive(existing.PID) {
		return false
	}
	return CheckReady(ctx, et.ResolvedReady, et.ResolvedDir) == nil
}

// processAlive reports whether pid refers to a live process, using
// signal 0 (no-op) which only checks existence/permission.
func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func (s *Scheduler) emit(name, msg string) {
	if s.Opts.Writer != nil {
		s.Opts.Writer(name, msg)
	}
}

func taskTimeout(et *EffectiveTask, fallback time.Duration) time.Duration {
	if et.Timeout != "" {
		if d, err := time.ParseDuration(et.Timeout); err == nil {
			return d
		}
	}
	return fallback
}

func sanitize(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		if r == ':' || r == '/' {
			out = append(out, '_')
			continue
		}
		out = append(out, r)
	}
	return string(out)
}
