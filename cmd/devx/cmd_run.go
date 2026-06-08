package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dever-labs/devx/internal/config"
)

func runRun(_ context.Context, args []string) error {
	manifest, err := loadManifestOnly()
	if err != nil {
		return err
	}

	if len(manifest.Scripts) == 0 {
		fmt.Println("No scripts defined in devx.yaml")
		fmt.Println()
		fmt.Println("Add a scripts section, for example:")
		fmt.Println()
		fmt.Println("  scripts:")
		fmt.Println("    - name: build")
		fmt.Println("      run: dotnet build")
		fmt.Println("      desc: Build the project")
		return nil
	}

	fs := flag.NewFlagSet("run", flag.ExitOnError)
	_ = fs.Parse(args)
	name := fs.Arg(0)

	// No name — list available scripts.
	if name == "" {
		return listScripts(manifest.Scripts)
	}

	// Find the named script.
	for _, s := range manifest.Scripts {
		if s.Name == name {
			return dispatchScript(s, fs.Args()[1:])
		}
	}

	return fmt.Errorf("script %q not found — run  devx run  to list available scripts", name)
}

// listScripts prints all scripts, noting which will run in the container.
func listScripts(scripts []config.Script) error {
	hasDevcontainer := devcontainerConfigExists()
	containerRunning := hasDevcontainer && isDevcontainerRunning()

	fmt.Println("Available scripts:")
	fmt.Println()

	maxLen := 0
	for _, s := range scripts {
		if len(s.Name) > maxLen {
			maxLen = len(s.Name)
		}
	}

	for _, s := range scripts {
		desc := s.Desc
		if desc == "" {
			desc = s.Run
			if len(desc) > 55 {
				desc = desc[:55] + "…"
			}
		}
		where := whereLabel(s, hasDevcontainer, containerRunning)
		fmt.Printf("  devx run %-*s  %-55s  %s\n", maxLen, s.Name, desc, where)
	}

	if hasDevcontainer && !containerRunning {
		fmt.Println()
		fmt.Println("  Devcontainer found but not running — scripts will run on host.")
		fmt.Println("  Start it with: devx dev up")
	}
	return nil
}

// whereLabel returns a short tag describing where a script will execute.
func whereLabel(s config.Script, hasDevcontainer, containerRunning bool) string {
	switch strings.ToLower(s.Container) {
	case "false":
		return "[host]"
	case "true":
		if containerRunning {
			return "[container]"
		}
		return "[container — not running]"
	default: // "auto" or ""
		if hasDevcontainer && containerRunning {
			return "[container]"
		}
		return "[host]"
	}
}

// dispatchScript decides whether to run on host or inside the devcontainer.
func dispatchScript(s config.Script, extraArgs []string) error {
	containerPref := strings.ToLower(s.Container)

	// container: false — always host.
	if containerPref == "false" {
		return execScriptOnHost(s, extraArgs)
	}

	hasDevcontainer := devcontainerConfigExists()

	// No devcontainer in this project at all.
	if !hasDevcontainer {
		if containerPref == "true" {
			return fmt.Errorf("script %q requires a devcontainer but no devcontainer.json found", s.Name)
		}
		return execScriptOnHost(s, extraArgs)
	}

	// Project has a devcontainer — check if it's running.
	if isDevcontainerRunning() {
		fmt.Printf("Running  %s  inside devcontainer...\n", s.Name)
		return execScriptInContainer(s, extraArgs)
	}

	// Container not running.
	if containerPref == "true" {
		return fmt.Errorf(
			"devcontainer is not running — start it with: devx dev up\n"+
				"  (script %q has container: true)", s.Name,
		)
	}

	// auto — fall back to host with a notice.
	fmt.Printf("Devcontainer not running — running  %s  on host.\n", s.Name)
	fmt.Println("  Start the container with  devx dev up  to run scripts inside it.")
	fmt.Println()
	return execScriptOnHost(s, extraArgs)
}

// execScriptOnHost runs the script command via the system shell on the host.
// It automatically loads .env from the script's workdir (or CWD) before running.
func execScriptOnHost(s config.Script, extraArgs []string) error {
	command := s.Run
	if len(extraArgs) > 0 {
		command += " " + strings.Join(extraArgs, " ")
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}

	if s.Workdir != "" {
		abs, err := filepath.Abs(s.Workdir)
		if err != nil {
			return err
		}
		cmd.Dir = abs
	}

	// Load .env and inject into the child process environment.
	env := os.Environ()
	for k, v := range loadDotEnvToMap(dotEnvPath(s.Workdir)) {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// execScriptInContainer runs the script via `devcontainer exec`.
func execScriptInContainer(s config.Script, extraArgs []string) error {
	command := s.Run
	if len(extraArgs) > 0 {
		command += " " + strings.Join(extraArgs, " ")
	}

	// Build the exec args: devcontainer exec --workspace-folder . -- sh -c "<cmd>"
	execArgs := []string{"exec", "--workspace-folder", "."}

	// If workdir is set, prefix the command with a cd.
	if s.Workdir != "" {
		abs, err := filepath.Abs(s.Workdir)
		if err != nil {
			return err
		}
		command = fmt.Sprintf("cd %q && %s", abs, command)
	}

	execArgs = append(execArgs, "--", "sh", "-c", command)

	cmd := exec.Command("devcontainer", execArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// devcontainerConfigExists returns true when .devcontainer/devcontainer.json
// or devcontainer.json exists in the current directory.
func devcontainerConfigExists() bool {
	for _, p := range []string{
		filepath.Join(".devcontainer", "devcontainer.json"),
		"devcontainer.json",
	} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// isDevcontainerRunning uses `docker ps` to check for a container labelled
// with the current workspace folder (how the devcontainer CLI tags containers).
func isDevcontainerRunning() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	abs, err := filepath.Abs(".")
	if err != nil {
		return false
	}
	filter := fmt.Sprintf("label=devcontainer.local_folder=%s", abs)
	out, err := exec.Command("docker", "ps", "--filter", filter, "--format", "{{.ID}}").Output()
	if err != nil {
		return false
	}
	return len(bytes.TrimSpace(out)) > 0
}

