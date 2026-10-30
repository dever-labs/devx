package localai

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"
)

var devcontainersScriptsBase = "https://raw.githubusercontent.com/dever-labs/devcontainers/main/scripts"

// SetupOptions configures a local AI backend setup run.
type SetupOptions struct {
	Backend           string // mlx | ollama | auto
	Model             string
	AutocompleteModel string
}

// Setup installs and starts the appropriate local AI backend.
// It shells out to the relevant host setup script from dever-labs/devcontainers.
func Setup(opts SetupOptions) error {
	backend := ResolveBackend(opts.Backend)

	switch backend {
	case BackendMLX:
		return setupMLX(opts.Model)
	case BackendOllama:
		return setupOllama(opts.Model)
	default:
		return fmt.Errorf("unknown backend: %s", backend)
	}
}

func setupMLX(model string) error {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return fmt.Errorf("MLX requires macOS with Apple Silicon — use 'devx ai setup --backend ollama' on this machine")
	}

	fmt.Println("Setting up MLX-LM (Apple Silicon inference)...")
	args := []string{}
	if model != "" {
		args = append(args, model)
	}
	return runRemoteScript("mlx-setup", args...)
}

func setupOllama(model string) error {
	fmt.Println("Setting up Ollama...")
	args := []string{}
	if model != "" {
		args = append(args, model)
	}
	return runRemoteScript("ollama-setup", args...)
}

// runRemoteScript fetches and executes a script from dever-labs/devcontainers.
func runRemoteScript(script string, args ...string) error {
	// Try local `dever` command first (available inside devcontainers).
	// Args are passed as real argv elements here, never through a shell,
	// so no quoting/injection concern applies to this path.
	if path, err := exec.LookPath("dever"); err == nil {
		cmdArgs := append([]string{script}, args...)
		cmd := exec.Command(path, cmdArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		return cmd.Run()
	}

	// Fall back to downloading the script and running it directly. The
	// script content is written to a local file and args are passed as
	// real argv elements to `bash <file> arg1 arg2 ...` -- never
	// interpolated into a shell command string -- so a value containing
	// shell metacharacters (e.g. `$(...)`) cannot be reinterpreted by a
	// shell and achieve command injection.
	scriptURL := fmt.Sprintf("%s/%s", devcontainersScriptsBase, script)
	scriptPath, err := downloadToTempFile(scriptURL)
	if err != nil {
		return fmt.Errorf("download setup script %q: %w", script, err)
	}
	defer os.Remove(scriptPath)

	// These scripts are authored for bash (same interpreter the previous
	// curl | bash implementation always used, on every GOOS) -- invoke it
	// directly rather than via a POSIX "sh", which may not support the
	// bashisms they rely on.
	bashArgs := append([]string{scriptPath}, args...)
	cmd := exec.Command("bash", bashArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// downloadToTempFile fetches url's body into a new temp file and returns
// its path. The caller is responsible for removing the file.
func downloadToTempFile(url string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d fetching %s", resp.StatusCode, url)
	}

	f, err := os.CreateTemp("", "devx-ai-setup-*.sh")
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
