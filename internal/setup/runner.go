// Package setup implements tool checking/installation and host-side setup steps
// for the `devx setup` and `devx doctor --fix` workflows.
package setup

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	goruntime "runtime"
	"strings"

	"github.com/dever-labs/devx/internal/config"
	"github.com/dever-labs/devx/internal/detect"
)

// Result describes the outcome of a single tool check or setup step.
type Result struct {
	Name    string `json:"name"`
	Type    string `json:"type"`   // "tool" | "step"
	Status  string `json:"status"` // "ok" | "installed" | "skipped" | "not-applicable" | "missing" | "failed"
	Detail  string `json:"detail,omitempty"`
	Source  string `json:"source,omitempty"` // where a tool version requirement came from
	Warning string `json:"warning,omitempty"`
}

// Options controls setup behaviour.
type Options struct {
	// Fix causes missing tools to be installed and runOnce steps to be re-run.
	Fix bool
	// Stdout is where subprocess stdout is routed. Defaults to os.Stdout when nil.
	// Set to os.Stderr in --json mode to keep JSON stdout clean.
	Stdout io.Writer
}

// stepOut returns the effective stdout destination for subprocess output.
func (o Options) stepOut() io.Writer {
	if o.Stdout != nil {
		return o.Stdout
	}
	return os.Stdout
}

// CheckTool runs the tool's Check command and reports whether the tool is present.
// Returns (true, versionOutput) on success, (false, errorDetail) on failure.
func CheckTool(tool config.Tool) (bool, string) {
	if tool.Check == "" {
		return false, "no check command defined"
	}
	out, err := runShellCapture(tool.Check)
	if err != nil {
		return false, strings.TrimSpace(out)
	}
	return true, strings.TrimSpace(out)
}

// InstallTool runs the platform-appropriate install command for the given tool.
// Subprocess stdout is routed via opts.Stdout (defaults to os.Stdout).
// Install progress lines are written to stderr so JSON stdout stays clean.
func InstallTool(_ context.Context, tool config.Tool, opts Options) error {
	installCmd := versionManagerInstall(tool)
	if installCmd == "" {
		installCmd = platformInstall(tool.Install)
	}
	if installCmd == "" {
		return fmt.Errorf("no install command defined for platform %s", goruntime.GOOS)
	}
	fmt.Fprintf(os.Stderr, "    $ %s\n", installCmd)
	return runShellInherit(installCmd, "", opts.stepOut())
}

// RunTools checks every declared tool and installs any that are missing when
// opts.Fix is true. Results are returned in declaration order. Missing
// version/check fields are filled from repo files (see ResolveTools).
// Progress messages are written to stderr so that --json stdout stays clean.
func RunTools(ctx context.Context, tools []config.Tool, opts Options) []Result {
	wd, _ := os.Getwd()
	var results []Result
	for _, tool := range ResolveTools(wd, tools) {
		r := runTool(ctx, tool, opts)
		r.Source = tool.Source
		switch {
		case tool.Conflict != "":
			r.Warning = tool.Conflict
		case tool.Version != "" && (r.Status == "ok" || r.Status == "installed") && !detect.VersionMatches(tool.Version, r.Detail):
			r.Warning = fmt.Sprintf("installed version does not match required %s (%s)", tool.Version, tool.Source)
		}
		results = append(results, r)
	}
	return results
}

func runTool(ctx context.Context, tool config.Tool, opts Options) Result {
	res := func(status, detail string) Result {
		return Result{Name: tool.Name, Type: "tool", Status: status, Detail: detail}
	}
	ok, detail := CheckTool(tool)
	if ok {
		return res("ok", detail)
	}
	if !opts.Fix {
		return res("missing", detail)
	}
	fmt.Fprintf(os.Stderr, "  Installing %s...\n", tool.Name)
	if err := InstallTool(ctx, tool, opts); err != nil {
		return res("failed", err.Error())
	}
	if ok2, detail2 := CheckTool(tool); ok2 {
		return res("installed", detail2)
	} else {
		return res("failed", "installed but check still fails: "+detail2)
	}
}

// RunSteps executes setup steps in declaration order.
// RunOnce steps whose command hash matches the stored state are skipped unless
// opts.Fix is true.
// Progress messages are written to stderr so that --json stdout stays clean.
func RunSteps(steps []config.SetupStep, opts Options) []Result {
	state := loadState()
	var results []Result
	for _, step := range steps {
		// Platform filter
		if !matchesPlatform(step.Platform) {
			results = append(results, Result{
				Name:   step.Name,
				Type:   "step",
				Status: "not-applicable",
				Detail: fmt.Sprintf("skipped (platform: %s)", step.Platform),
			})
			continue
		}
		// RunOnce idempotency check
		if step.RunOnce && !opts.Fix {
			if hasRunBefore(state, step.Name, step.Run, step.Workdir) {
				results = append(results, Result{
					Name:   step.Name,
					Type:   "step",
					Status: "skipped",
					Detail: "already run (runOnce)",
				})
				continue
			}
		}
		fmt.Fprintf(os.Stderr, "  [%s] %s\n", step.Name, step.Run)
		if err := runShellInherit(step.Run, step.Workdir, opts.stepOut()); err != nil {
			results = append(results, Result{
				Name:   step.Name,
				Type:   "step",
				Status: "failed",
				Detail: err.Error(),
			})
			continue
		}
		markDone(state, step.Name, step.Run, step.Workdir)
		results = append(results, Result{
			Name:   step.Name,
			Type:   "step",
			Status: "ok",
		})
	}
	_ = saveState(state)
	return results
}

// platformInstall returns the install command for the current OS.
func platformInstall(install config.Install) string {
	switch goruntime.GOOS {
	case "windows":
		return install.Windows
	case "darwin":
		return install.MacOS
	default:
		return install.Linux
	}
}

// matchesPlatform reports whether the given platform tag matches the current OS.
// Returns false for unrecognised values — ValidateSetup is the correct gate.
func matchesPlatform(platform string) bool {
	switch platform {
	case "", "all":
		return true
	case "windows":
		return goruntime.GOOS == "windows"
	case "macos", "darwin":
		return goruntime.GOOS == "darwin"
	case "linux":
		return goruntime.GOOS == "linux"
	}
	return false
}

// runShellCapture runs a shell command and returns combined output + error.
func runShellCapture(command string) (string, error) {
	var cmd *exec.Cmd
	if goruntime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// runShellInherit runs a shell command with stdout routed to the given writer
// and stderr inherited from the parent process (visible to user).
func runShellInherit(command, workdir string, stdout io.Writer) error {
	var cmd *exec.Cmd
	if goruntime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	if workdir != "" {
		cmd.Dir = workdir
	}
	cmd.Stdout = stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
