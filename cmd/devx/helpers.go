package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	devxai "github.com/dever-labs/devx/internal/ai"
	"github.com/dever-labs/devx/internal/compose"
	"github.com/dever-labs/devx/internal/config"
	"github.com/dever-labs/devx/internal/graph"
	"github.com/dever-labs/devx/internal/lock"
	"github.com/dever-labs/devx/internal/providers"
	devxruntime "github.com/dever-labs/devx/internal/runtime"
	"github.com/dever-labs/devx/internal/runtime/docker"
	"github.com/dever-labs/devx/internal/runtime/podman"
)

// loadManifestOnly loads and parses devx.yaml without resolving a profile.
// Use this when a command doesn't need a specific profile (e.g. setup, validate).
func loadManifestOnly() (*config.Manifest, error) {
	return config.Load(manifestFile)
}

func loadProfile(profile string) (*config.Manifest, string, *config.Profile, error) {
	manifest, err := config.Load(manifestFile)
	if err != nil {
		return nil, "", nil, err
	}

	if err := config.Validate(manifest); err != nil {
		return nil, "", nil, err
	}

	profName := profile
	if profName == "" {
		profName = manifest.Project.DefaultProfile
	}
	if profName == "" {
		return nil, "", nil, fmt.Errorf("no profile specified and no defaultProfile set in manifest")
	}

	prof, err := config.ProfileByName(manifest, profName)
	if err != nil {
		return nil, "", nil, err
	}

	if err := config.ValidateProfile(manifest, profName); err != nil {
		return nil, "", nil, err
	}

	return manifest, profName, prof, nil
}

func writeCompose(path string, manifest *config.Manifest, profName string, prof *config.Profile, lockfile *lock.Lockfile, enableTelemetry bool) error {
	composed, err := buildCompose(manifest, profName, prof, lockfile, enableTelemetry)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(composed), 0600); err != nil {
		return err
	}

	assets := compose.TelemetryAssets(enableTelemetry)
	if len(assets) == 0 {
		return nil
	}

	baseDir := filepath.Dir(path)
	for _, asset := range assets {
		assetPath := filepath.Join(baseDir, asset.Path)
		if err := os.MkdirAll(filepath.Dir(assetPath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(assetPath, asset.Content, 0644); err != nil {
			return err
		}
	}

	return nil
}

func buildCompose(manifest *config.Manifest, profName string, prof *config.Profile, lockfile *lock.Lockfile, enableTelemetry bool) (string, error) {
	prof = resolveDepImages(prof)
	prof = resolveConnections(manifest, prof)

	g, err := graph.Build(prof)
	if err != nil {
		return "", err
	}
	if _, err := graph.TopoSort(g); err != nil {
		return "", err
	}

	depFragments := buildDepFragments(prof)

	rewrite := compose.RewriteOptions{
		RegistryPrefix: manifest.Registry.Prefix,
		Lockfile:       lockfile,
		DepFragments:   depFragments,
	}

	return compose.Render(manifest, profName, prof, rewrite, enableTelemetry)
}

func profileRuntime(prof *config.Profile) string {
	if prof == nil || prof.Runtime == "" {
		return "compose"
	}
	return prof.Runtime
}

func ensureDevxDir() error {
	return os.MkdirAll(devxDir, 0755)
}

func ensureGitignore() error {
	path := ".gitignore"
	entry := ".devx/"

	if !fileExists(path) {
		return os.WriteFile(path, []byte(entry+"\n"), 0644)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if strings.Contains(string(data), entry) {
		return nil
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString("\n" + entry + "\n")
	return err
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeState(s state) error {
	if err := ensureDevxDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(devxDir, stateFile), data, 0600)
}

func readState() *state {
	data, err := os.ReadFile(filepath.Join(devxDir, stateFile))
	if err != nil {
		return nil
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return nil
	}
	return &s
}

func telemetryFromState() bool {
	st := readState()
	if st == nil {
		return false
	}
	return st.Telemetry
}

func streamLogs(reader io.Reader, jsonOut bool) error {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		if !jsonOut {
			fmt.Println(line)
			continue
		}
		entry := map[string]string{"line": line}
		data, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	}
	return scanner.Err()
}

func collectImages(manifest *config.Manifest, profileName string, prof *config.Profile) ([]string, error) {
	composed, err := buildCompose(manifest, profileName, prof, nil, true)
	if err != nil {
		return nil, err
	}

	imgs, err := compose.CollectImages([]byte(composed))
	if err != nil {
		return nil, err
	}

	return imgs, nil
}

func selectRuntime(ctx context.Context) (devxruntime.Runtime, error) {
	d := docker.New()
	if ok, _ := d.Detect(ctx); ok {
		return d, nil
	}
	p := podman.New()
	if ok, _ := p.Detect(ctx); ok {
		return p, nil
	}
	return nil, devxruntime.ErrNoRuntime
}

// printLinks queries the running stack for actual host-port bindings and prints
// http://localhost:<port> for every published port. Using the runtime (not the
// compose YAML) ensures randomly-assigned ports are reflected correctly.
func printLinks(ctx context.Context, rt devxruntime.Runtime, composePath, projectName string) {
	statuses, err := rt.Status(ctx, composePath, projectName)
	if err != nil {
		return
	}

	type link struct {
		label string
		url   string
	}
	var links []link
	seen := map[string]bool{}

	for _, svc := range statuses {
		label := serviceLabel(svc.Name)
		for _, pub := range svc.Publishers {
			if pub.PublishedPort == 0 {
				continue
			}
			url := fmt.Sprintf("http://localhost:%d", pub.PublishedPort)
			key := label + url
			if seen[key] {
				continue
			}
			seen[key] = true
			links = append(links, link{label: label, url: url})
		}
	}

	if len(links) == 0 {
		return
	}

	sort.Slice(links, func(i, j int) bool { return links[i].label < links[j].label })

	fmt.Println("\nAvailable services:")
	maxLen := 0
	for _, l := range links {
		if len(l.label) > maxLen {
			maxLen = len(l.label)
		}
	}
	for _, l := range links {
		fmt.Printf("  %-*s  %s\n", maxLen, l.label, l.url)
	}
}

// wellKnownLabels maps telemetry service name suffixes to display labels.
var wellKnownLabels = map[string]string{
	"grafana":     "Grafana",
	"prometheus":  "Prometheus",
	"loki":        "Loki",
	"cadvisor":    "cAdvisor",
	"alloy":       "Alloy",
	"docker-meta": "Docker Meta",
}

func serviceLabel(name string) string {
	lower := strings.ToLower(name)
	// Strip common telemetry prefix
	stripped := strings.TrimPrefix(lower, "devx-telemetry-")
	if label, ok := wellKnownLabels[stripped]; ok {
		return label
	}
	// Title-case the service name for user services
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' })
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// runHooks executes a slice of lifecycle hooks sequentially, stopping on first error.
// exec hooks run a command inside a running container; run hooks run a host shell command.
// run hooks with background:true are started without waiting and their *exec.Cmd is
// returned so the caller can stream output and wait on them later.
func runHooks(ctx context.Context, rt devxruntime.Runtime, composePath, projectName string, hooks []config.Hook) ([]*exec.Cmd, error) {
	var bgCmds []*exec.Cmd
	for i, h := range hooks {
		if h.Exec != "" {
			fmt.Printf("  [hook %d] exec in %s: %s\n", i+1, h.Service, h.Exec)
			cmd := strings.Fields(h.Exec)
			code, err := rt.Exec(ctx, composePath, projectName, h.Service, cmd)
			if err != nil {
				return bgCmds, fmt.Errorf("hook %d exec failed: %w", i+1, err)
			}
			if code != 0 {
				return bgCmds, fmt.Errorf("hook %d exec exited with code %d", i+1, code)
			}
		} else if h.Background {
			label := h.Name
			if label == "" {
				label = h.Run
				if len(label) > 40 {
					label = label[:40]
				}
			}
			fmt.Printf("  [hook %d] run (background): %s\n", i+1, h.Run)
			cmd, err := runShellCommandBackground(h.Run, label)
			if err != nil {
				return bgCmds, fmt.Errorf("hook %d background run failed: %w", i+1, err)
			}
			bgCmds = append(bgCmds, cmd)
		} else {
			fmt.Printf("  [hook %d] run: %s\n", i+1, h.Run)
			if err := runShellCommand(h.Run); err != nil {
				return bgCmds, fmt.Errorf("hook %d run failed: %w", i+1, err)
			}
		}
	}
	return bgCmds, nil
}

// runShellCommand runs a command string via the system shell with stdout/stderr inherited.
func runShellCommand(command string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runShellCommandBackground starts a command via the system shell without waiting for it
// to exit. stdout and stderr are streamed to the terminal with a label prefix.
func runShellCommandBackground(command, label string) (*exec.Cmd, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	prefix := fmt.Sprintf("[%s] ", label)
	go streamWithPrefix(stdout, os.Stdout, prefix)
	go streamWithPrefix(stderr, os.Stderr, prefix)

	return cmd, nil
}

// streamWithPrefix reads lines from r and writes them to w with a label prefix.
func streamWithPrefix(r io.Reader, w io.Writer, prefix string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fmt.Fprintf(w, "%s%s\n", prefix, scanner.Text())
	}
}

// waitForBackground blocks until all background processes exit or the user interrupts.
// On interrupt, each process receives SIGINT and is force-killed after a short grace period.
func waitForBackground(cmds []*exec.Cmd) {
	if len(cmds) == 0 {
		return
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	doneCh := make(chan struct{}, len(cmds))
	for _, cmd := range cmds {
		cmd := cmd
		go func() {
			cmd.Wait() //nolint:errcheck // exit code of background process is informational
			doneCh <- struct{}{}
		}()
	}

	remaining := len(cmds)
	for {
		select {
		case <-sigCh:
			fmt.Println("\nStopping background processes...")
			for _, cmd := range cmds {
				if cmd.Process != nil {
					cmd.Process.Signal(os.Interrupt) //nolint:errcheck
				}
			}
			// Grace period, then force kill.
			time.Sleep(3 * time.Second)
			for _, cmd := range cmds {
				if cmd.Process != nil {
					cmd.Process.Kill() //nolint:errcheck
				}
			}
			return
		case <-doneCh:
			remaining--
			if remaining == 0 {
				return
			}
		}
	}
}

// loadLockfile loads devx.lock if it exists, returning nil without error when absent.
func loadLockfile() (*lock.Lockfile, error) {
	lf, err := lock.Load(lockFile)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return lf, err
}

// ensureProviders fetches any provider binaries declared inline in the profile
// deps that are not yet cached locally. Prints a warning for each provider that
// cannot be fetched but does not return an error.
func ensureProviders(manifest *config.Manifest, prof *config.Profile) {
	lf, _ := loadLockfile()
	seen := map[string]bool{}

	for _, dep := range prof.Deps {
		if dep.Kind == "" || dep.Version == "" {
			continue
		}
		src := depProviderSource(dep)
		key := src + "@" + dep.Version
		if seen[key] {
			continue
		}
		seen[key] = true

		cached, _ := providers.IsCached(src, dep.Version)
		if cached {
			continue
		}
		fmt.Printf("Fetching provider %q (%s@%s)...\n", dep.Kind, src, dep.Version)
		resolvedVersion, digest, err := providers.Fetch(src, dep.Version)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not fetch provider %q: %v\n", dep.Kind, err)
			continue
		}
		if lf != nil {
			if pin, ok := lf.Providers[src]; ok && pin.SHA256 != "" {
				if verifyErr := providers.VerifyDigest(src, resolvedVersion, pin.SHA256); verifyErr != nil {
					fmt.Fprintf(os.Stderr, "warning: %v\n", verifyErr)
				}
			}
		}
		fmt.Printf("  ✓ %s@%s (sha256:%s)\n", src, resolvedVersion, digest[:12])
	}
}

// buildDepFragments invokes each dep's provider (if declared and cached) to
// retrieve optional compose fragments such as healthcheck definitions.
func buildDepFragments(prof *config.Profile) map[string]*compose.DepFragment {
	fragments := map[string]*compose.DepFragment{}
	for name, dep := range prof.Deps {
		if dep.Kind == "" || dep.Version == "" {
			continue
		}
		src := depProviderSource(dep)
		input := providers.DepInput{
			Name:   name,
			Image:  dep.Image,
			Ports:  dep.Ports,
			Env:    dep.Env,
			Volume: dep.Volume,
		}
		raw, err := providers.InvokeRenderCompose(src, dep.Version, input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: provider %q render-compose failed: %v\n", dep.Kind, err)
			continue
		}
		if raw == nil || raw.Healthcheck == nil {
			continue
		}
		fragments[name] = &compose.DepFragment{
			Healthcheck: &compose.Healthcheck{
				Test:     raw.Healthcheck.Test,
				Interval: raw.Healthcheck.Interval,
				Retries:  raw.Healthcheck.Retries,
			},
		}
	}
	return fragments
}

// resolveDepImages returns a shallow copy of prof with missing dep images
// filled in from the provider's defaultImage (via describe).
func resolveDepImages(prof *config.Profile) *config.Profile {
	resolvedDeps := make(map[string]config.Dep, len(prof.Deps))
	for name, dep := range prof.Deps {
		resolvedDeps[name] = dep
	}

	for name, dep := range resolvedDeps {
		if dep.Image != "" || dep.Kind == "" || dep.Version == "" {
			continue
		}
		src := depProviderSource(dep)
		meta, err := providers.Describe(src, dep.Version)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: provider %q describe failed: %v\n", dep.Kind, err)
			continue
		}
		if meta == nil {
			fmt.Fprintf(os.Stderr, "warning: provider %q is not installed — run 'devx providers install'\n", dep.Kind)
			continue
		}
		if meta.DefaultImage == "" {
			fmt.Fprintf(os.Stderr, "warning: provider %q did not return a defaultImage\n", dep.Kind)
			continue
		}
		dep.Image = meta.DefaultImage
		resolvedDeps[name] = dep
	}

	resolved := *prof
	resolved.Deps = resolvedDeps
	return &resolved
}

// resolveConnections returns a shallow copy of prof with dep connect entries
// applied — injecting the appropriate env vars into the target service containers.
//
// For each dep.connect entry:
//   - If Env is set, template variables (${host}, ${port}, ${<DEP_ENV_KEY>}) are
//     substituted with actual values and the result is merged into the service env.
//   - If Env is omitted and AI is configured, the LLM scans the service build context
//     to detect the correct env var names automatically.
//   - If neither applies, a warning is printed and the entry is skipped.
//
// Service env vars that are already explicitly set are NOT overridden.
func resolveConnections(manifest *config.Manifest, prof *config.Profile) *config.Profile {
	hasConnect := false
	for _, dep := range prof.Deps {
		if len(dep.Connect) > 0 {
			hasConnect = true
			break
		}
	}
	if !hasConnect {
		return prof
	}

	// Shallow-copy services map so we can safely mutate env maps.
	services := make(map[string]config.Service, len(prof.Services))
	for name, svc := range prof.Services {
		// Deep-copy the env map for this service.
		env := make(map[string]string, len(svc.Env))
		for k, v := range svc.Env {
			env[k] = v
		}
		svc.Env = env
		services[name] = svc
	}

	var aiClient *devxai.Client
	if manifest.AI != nil {
		var err error
		aiClient, err = devxai.New(manifest.AI.Provider, manifest.AI.Model, manifest.AI.BaseURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: AI not available for connection detection: %v\n", err)
		}
	}

	for depName, dep := range prof.Deps {
		if len(dep.Connect) == 0 {
			continue
		}
		outputVals := providers.ResolveOutputValues(depName, dep.Ports, dep.Env)

		for _, entry := range dep.Connect {
			svc, ok := services[entry.Service]
			if !ok {
				fmt.Fprintf(os.Stderr, "warning: dep %q connect: service %q not found\n", depName, entry.Service)
				continue
			}

			vals := outputVals
			if svc.IsProcess() {
				vals = hostVals(outputVals, dep.Ports)
			}

			var injected map[string]string
			if len(entry.Env) > 0 {
				injected = renderEnvTemplates(entry.Env, vals)
			} else if aiClient != nil {
				var serviceDir string
				if svc.Build != nil {
					serviceDir = svc.Build.Context
				}
				var err error
				injected, err = devxai.Detect(context.Background(), aiClient, dep.Kind, vals, serviceDir)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: AI detection for dep %q → service %q failed: %v\n", depName, entry.Service, err)
					continue
				}
			} else {
				fmt.Fprintf(os.Stderr, "warning: dep %q connect to %q has no env mapping and AI is not configured — add an 'ai' block to devx.yaml or provide manual env mapping\n", depName, entry.Service)
				continue
			}

			// Inject — service's own explicit env vars always win.
			for k, v := range injected {
				if _, alreadySet := svc.Env[k]; !alreadySet {
					svc.Env[k] = v
				}
			}
			services[entry.Service] = svc
		}
	}

	resolved := *prof
	resolved.Services = services
	return &resolved
}

// renderEnvTemplates substitutes ${key} placeholders in each value of the env
// map using the provided outputVals lookup. Unknown keys are left as-is.
func renderEnvTemplates(env map[string]string, outputVals map[string]string) map[string]string {
	result := make(map[string]string, len(env))
	for k, v := range env {
		result[k] = expandTemplate(v, outputVals)
	}
	return result
}

// expandTemplate replaces ${key} occurrences in s with values from vars.
func expandTemplate(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
	}
	return s
}

// depProviderSource returns the effective provider source for a dep.
// If dep.Source is set it is used as-is; otherwise it defaults to
// "devx-labs/<kind>".
func depProviderSource(dep config.Dep) string {
	if dep.Source != "" {
		return dep.Source
	}
	return "devx-labs/" + dep.Kind
}
