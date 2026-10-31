// Package task implements the devx task execution engine: ${VAR}
// substitution, readiness/detached/port inference from run: text, the
// Driver/Handle abstraction over processes and containers, and the
// scheduler that walks the dependency graph to run devx up/run/down.
package task

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/dever-labs/devx/internal/config"
)

// varPattern matches ${VAR} (not $VAR, and not the $$ escape, which is
// handled separately before this pattern ever runs).
var varPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// dollarEscape is the Make-style literal-dollar escape: "$${VAR}" means
// the literal text "${VAR}", not substitution.
const dollarEscapePlaceholder = "\x00DOLLAR\x00"

// Resolver resolves ${VAR} references for one task's scope, following the
// documented precedence: CLI override > inherited process env > .env/
// .env.local > vars: defaults (task's own file scope, root as fallback).
type Resolver struct {
	cliOverrides map[string]string
	dotEnv       map[string]string
	scopeVars    map[string]string
}

// NewResolver builds a Resolver for one task, given CLI KEY=VALUE
// overrides (shared across every task in the invocation), the process
// environment (read live from os.Environ via Lookup), the parsed .env/
// .env.local file(s) (already merged, .env.local winning), and the
// task's own resolved ScopeVars.
func NewResolver(cliOverrides, dotEnv, scopeVars map[string]string) *Resolver {
	return &Resolver{cliOverrides: cliOverrides, dotEnv: dotEnv, scopeVars: scopeVars}
}

func (r *Resolver) lookup(name string) (string, bool) {
	if v, ok := r.cliOverrides[name]; ok {
		return v, true
	}
	if v, ok := os.LookupEnv(name); ok {
		return v, true
	}
	if v, ok := r.dotEnv[name]; ok {
		return v, true
	}
	if v, ok := r.scopeVars[name]; ok {
		return v, true
	}
	return "", false
}

// Substitute resolves every ${VAR} in s, honoring the $${VAR} -> literal
// "${VAR}" escape. Unknown variables are left as an error, since a typo'd
// ${VAR} silently becoming empty text is a worse failure mode than a
// clear error at substitution time.
func (r *Resolver) Substitute(s string) (string, error) {
	escaped := strings.ReplaceAll(s, "$$", dollarEscapePlaceholder)

	var firstErr error
	out := varPattern.ReplaceAllStringFunc(escaped, func(match string) string {
		name := match[2 : len(match)-1]
		v, ok := r.lookup(name)
		if !ok {
			if firstErr == nil {
				firstErr = fmt.Errorf("undefined variable ${%s}", name)
			}
			return match
		}
		return v
	})
	if firstErr != nil {
		return "", firstErr
	}
	out = strings.ReplaceAll(out, dollarEscapePlaceholder, "$")
	return out, nil
}

// ParseDotEnv parses a .env-style file (KEY=VALUE per line, '#' comments,
// blank lines ignored). Values are not shell-unescaped beyond simple
// surrounding-quote stripping.
func ParseDotEnv(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		val = strings.Trim(val, `"'`)
		out[key] = val
	}
	return out, nil
}

// LoadDotEnv loads .env then .env.local from dir, merging with .env.local
// taking precedence, per the documented root-only discovery rule.
func LoadDotEnv(dir string) (map[string]string, error) {
	base, err := ParseDotEnv(filepath.Join(dir, ".env"))
	if err != nil {
		return nil, err
	}
	local, err := ParseDotEnv(filepath.Join(dir, ".env.local"))
	if err != nil {
		return nil, err
	}
	for k, v := range local {
		base[k] = v
	}
	return base, nil
}

// portPattern matches docker/CLI-style -p/--port host-port mappings,
// e.g. "-p 5432:5432", "--port=3000", "-p 127.0.0.1:8080:80". The whole
// value after the flag is captured; see portPatternHostPort for how the
// host-reachable port is picked out of it ([ip:]hostPort:containerPort).
var portPattern = regexp.MustCompile(`(?:-p|--publish|--port)[= ]"?([\w.:]+)`)

// namePattern matches docker-style --name/-n container name flags.
var namePattern = regexp.MustCompile(`(?:--name|-n)[= ]([\w.\-]+)`)

// hostPortFromMapping extracts the host-reachable port from a docker -p
// value, which is one of: "containerPort", "hostPort:containerPort", or
// "ip:hostPort:containerPort" (any segment may carry a trailing
// "/tcp"/"/udp" protocol suffix, which only ever appears on the last
// segment). The host port is the second-to-last colon-separated field
// when there is more than one field, otherwise the only field.
func hostPortFromMapping(value string) (int, bool) {
	parts := strings.Split(value, ":")
	idx := 0
	if len(parts) > 1 {
		idx = len(parts) - 2
	}
	port := strings.SplitN(parts[idx], "/", 2)[0]
	p, err := strconv.Atoi(port)
	if err != nil {
		return 0, false
	}
	return p, true
}

// containerPortFromMapping extracts the *container-internal* port from
// the same docker -p value hostPortFromMapping reads — always the last
// colon-separated field. This is what's actually reachable when devx
// probes the container directly by IP (see readyWithContainerIP):
// container IP:hostPort rarely works, since hostPort is only published on
// the real host, not on the container's own listening socket.
func containerPortFromMapping(value string) (int, bool) {
	parts := strings.Split(value, ":")
	last := strings.SplitN(parts[len(parts)-1], "/", 2)[0]
	p, err := strconv.Atoi(last)
	if err != nil {
		return 0, false
	}
	return p, true
}

// InferredShape is what devx infers about a task purely from its run:
// text, before any explicit field overrides are applied.
type InferredShape struct {
	Port          int
	ContainerPort int
	ContainerName string
	IsContainer   bool
	Detached      bool
}

// Infer inspects a substituted run: command for a port, a container name,
// and whether the command looks like a container launch, to drive the
// detached/readiness/Stop() defaults documented in the design doc.
func Infer(run string) InferredShape {
	var shape InferredShape

	if m := portPattern.FindStringSubmatch(run); m != nil {
		if p, ok := hostPortFromMapping(m[1]); ok {
			shape.Port = p
		}
		if cp, ok := containerPortFromMapping(m[1]); ok {
			shape.ContainerPort = cp
		}
	}
	if m := namePattern.FindStringSubmatch(run); m != nil {
		shape.ContainerName = m[1]
	}
	if strings.Contains(run, "docker run") || strings.Contains(run, "docker compose") || strings.Contains(run, "podman run") {
		shape.IsContainer = true
	}
	if strings.Contains(run, " -d ") || strings.HasSuffix(run, " -d") || strings.Contains(run, "--detach") {
		shape.Detached = true
	}
	if shape.Port > 0 {
		shape.Detached = true
	}
	return shape
}

// EffectiveTask is a Task after ${VAR} substitution and inference have
// been fully resolved — what the scheduler and drivers actually act on.
type EffectiveTask struct {
	config.Task
	ResolvedRun   string
	ResolvedEnv   map[string]string
	ResolvedDir   string
	ResolvedDesc  string
	ResolvedReady *config.ReadyCheck
	Port          int
	ContainerPort int
	Detached      bool
	ContainerName string
	IsContainer   bool
}

// Resolve substitutes every documented string field of t and applies the
// inference rules, returning the fully effective task.
func Resolve(t config.Task, r *Resolver) (*EffectiveTask, error) {
	run, err := r.Substitute(t.Run)
	if err != nil {
		return nil, fmt.Errorf("task %q: run: %w", t.Name, err)
	}

	dir := t.Dir
	if dir == "" {
		dir = t.SourceDir
	} else if !filepath.IsAbs(dir) {
		dir = filepath.Join(t.SourceDir, dir)
	}
	dir, err = r.Substitute(dir)
	if err != nil {
		return nil, fmt.Errorf("task %q: dir: %w", t.Name, err)
	}

	env := map[string]string{}
	for k, v := range t.Env {
		rv, err := r.Substitute(v)
		if err != nil {
			return nil, fmt.Errorf("task %q: env.%s: %w", t.Name, k, err)
		}
		env[k] = rv
	}

	shape := Infer(run)

	containerName := shape.ContainerName
	if t.ContainerName != "" {
		containerName, err = r.Substitute(t.ContainerName)
		if err != nil {
			return nil, fmt.Errorf("task %q: containerName: %w", t.Name, err)
		}
	}

	port := shape.Port
	if t.Port != 0 {
		port = t.Port
	}

	var ready *config.ReadyCheck
	if t.Ready != nil {
		ready = &config.ReadyCheck{}
		if ready.HTTP, err = r.Substitute(t.Ready.HTTP); err != nil {
			return nil, fmt.Errorf("task %q: ready.http: %w", t.Name, err)
		}
		if ready.TCP, err = r.Substitute(t.Ready.TCP); err != nil {
			return nil, fmt.Errorf("task %q: ready.tcp: %w", t.Name, err)
		}
		if ready.Cmd, err = r.Substitute(t.Ready.Cmd); err != nil {
			return nil, fmt.Errorf("task %q: ready.cmd: %w", t.Name, err)
		}
	} else if port > 0 {
		ready = &config.ReadyCheck{HTTP: fmt.Sprintf("http://localhost:%d/healthz", port)}
	}

	detached := shape.Detached || port > 0 || t.Ready != nil
	if t.Detached != nil {
		detached = *t.Detached
	}

	// desc: is purely informational (shown in `devx tasks`/`devx status`),
	// so an undefined ${VAR} there shouldn't fail resolution the way it
	// would in run:/env:/ready: — fall back to the raw text instead.
	desc := t.Desc
	if resolvedDesc, derr := r.Substitute(t.Desc); derr == nil {
		desc = resolvedDesc
	}

	return &EffectiveTask{
		Task:          t,
		ResolvedRun:   run,
		ResolvedEnv:   env,
		ResolvedDir:   dir,
		ResolvedDesc:  desc,
		ResolvedReady: ready,
		Port:          port,
		ContainerPort: shape.ContainerPort,
		Detached:      detached,
		ContainerName: containerName,
		IsContainer:   shape.IsContainer,
	}, nil
}

// RedactedEnv renders env for --dry-run/--json/tasks output: key names
// shown, values always redacted, since env: values commonly carry
// secrets that must never echo back (see design doc "Secrets never echo
// back").
func RedactedEnv(env map[string]string) map[string]string {
	out := make(map[string]string, len(env))
	for k := range env {
		out[k] = "<redacted>"
	}
	return out
}
