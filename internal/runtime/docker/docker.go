package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/dever-labs/devx/internal/runtime"
)

type Runtime struct {
	Binary string
}

func New() *Runtime {
	return &Runtime{Binary: "docker"}
}

func (r *Runtime) Name() string {
	return "docker"
}

func (r *Runtime) Detect(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, r.Binary, "version", "--format", "{{.Server.Version}}")
	out, err := cmd.Output()
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func (r *Runtime) Up(ctx context.Context, composePath string, projectName string, opts runtime.UpOptions) error {
	args := []string{"compose", "-f", composePath, "-p", projectName, "up", "-d"}
	if opts.Build {
		args = append(args, "--build")
	}
	if opts.Pull {
		args = append(args, "--pull", "always")
	}
	return run(ctx, r.Binary, args...)
}

func (r *Runtime) Down(ctx context.Context, composePath string, projectName string, removeVolumes bool) error {
	args := []string{"compose", "-f", composePath, "-p", projectName, "down"}
	if removeVolumes {
		args = append(args, "--volumes")
	}
	return run(ctx, r.Binary, args...)
}

func (r *Runtime) Logs(ctx context.Context, composePath string, projectName string, opts runtime.LogsOptions) (io.ReadCloser, error) {
	args := []string{"compose", "-f", composePath, "-p", projectName, "logs", "--timestamps"}
	if opts.Follow {
		args = append(args, "--follow")
	}
	if opts.Since != "" {
		args = append(args, "--since", opts.Since)
	}
	if opts.Service != "" {
		args = append(args, opts.Service)
	}

	cmd := exec.CommandContext(ctx, r.Binary, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return newCommandReader(cmd, stdout), nil
}

func (r *Runtime) Exec(ctx context.Context, composePath string, projectName string, service string, cmdArgs []string) (int, error) {
	args := []string{"compose", "-f", composePath, "-p", projectName, "exec", "-T", service}
	args = append(args, cmdArgs...)
	cmd := exec.CommandContext(ctx, r.Binary, args...)
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}

func parseStatusEntries(out []byte) ([]map[string]any, error) {
	// Docker Compose v2 outputs NDJSON (one object per line), not a JSON array.
	// Fall back to array parsing for older versions.
	var entries []map[string]any
	if err := json.Unmarshal(out, &entries); err == nil {
		return entries, nil
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (r *Runtime) Status(ctx context.Context, composePath string, projectName string) ([]runtime.ServiceStatus, error) {
	args := []string{"compose", "-f", composePath, "-p", projectName, "ps", "--format", "json"}
	cmd := exec.CommandContext(ctx, r.Binary, args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	entries, err := parseStatusEntries(out)
	if err != nil {
		return nil, err
	}

	var results []runtime.ServiceStatus
	for _, entry := range entries {
		name, _ := entry["Service"].(string)
		state, _ := entry["State"].(string)
		health, _ := entry["Health"].(string)
		ports := fmt.Sprintf("%v", entry["Publishers"])

		var publishers []runtime.Publisher
		if raw, ok := entry["Publishers"]; ok {
			if data, err := json.Marshal(raw); err == nil {
				_ = json.Unmarshal(data, &publishers)
			}
		}

		results = append(results, runtime.ServiceStatus{
			Name:       name,
			State:      state,
			Health:     health,
			Ports:      ports,
			Publishers: publishers,
		})
	}

	return results, nil
}

func (r *Runtime) ResolveImageDigest(ctx context.Context, image string) (string, error) {
	digest, err := resolveRepoDigest(ctx, r.Binary, image)
	if err == nil {
		return digest, nil
	}

	if err := run(ctx, r.Binary, "pull", image); err != nil {
		return "", err
	}

	return resolveRepoDigest(ctx, r.Binary, image)
}

func resolveRepoDigest(ctx context.Context, binary string, image string) (string, error) {
	cmd := exec.CommandContext(ctx, binary, "image", "inspect", "--format", "{{join .RepoDigests \"\\n\"}}", image)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "@")
		if len(parts) == 2 {
			return parts[1], nil
		}
	}
	return "", fmt.Errorf("no digest found for %s", image)
}

func run(ctx context.Context, binary string, args ...string) error {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type commandReader struct {
	cmd *exec.Cmd
	rc  io.ReadCloser
}

func newCommandReader(cmd *exec.Cmd, rc io.ReadCloser) io.ReadCloser {
	return &commandReader{cmd: cmd, rc: rc}
}

func (c *commandReader) Read(p []byte) (int, error) {
	return c.rc.Read(p)
}

func (c *commandReader) Close() error {
	_ = c.cmd.Process.Kill()
	return c.rc.Close()
}
