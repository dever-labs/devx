package localai

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

const devcontainersScriptsBase = "https://raw.githubusercontent.com/dever-labs/devcontainers/main/scripts"

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
	// Try local `dever` command first (available inside devcontainers)
	if path, err := exec.LookPath("dever"); err == nil {
		cmdArgs := append([]string{script}, args...)
		cmd := exec.Command(path, cmdArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		return cmd.Run()
	}

	// Fall back to curl | bash
	scriptURL := fmt.Sprintf("%s/%s", devcontainersScriptsBase, script)
	curlArgs := fmt.Sprintf("curl -fsSL %s | bash", scriptURL)
	if len(args) > 0 {
		curlArgs = fmt.Sprintf("curl -fsSL %s | bash -s %s", scriptURL, joinArgs(args))
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", curlArgs)
	} else {
		cmd = exec.Command("sh", "-c", curlArgs)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func joinArgs(args []string) string {
	result := ""
	for i, a := range args {
		if i > 0 {
			result += " "
		}
		result += fmt.Sprintf("%q", a)
	}
	return result
}
