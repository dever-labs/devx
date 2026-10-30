package setup

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/dever-labs/devx/internal/config"
)

// RunEnvFiles scaffolds every declared envFiles entry relative to dir (the
// manifest's own directory). A file whose Output already exists is left
// untouched (status "skipped") — scaffolding only ever creates a file, so a
// developer's already-filled-in local secrets/config are never clobbered by
// a later `devx init` run, even with --fix.
func RunEnvFiles(dir string, files []config.EnvFile, opts Options) []Result {
	var results []Result
	for _, f := range files {
		results = append(results, runEnvFile(dir, f, opts))
	}
	return results
}

func runEnvFile(dir string, f config.EnvFile, opts Options) Result {
	name := f.Name
	if name == "" {
		name = f.Output
	}

	outPath := filepath.Join(dir, f.Output)
	if _, err := os.Stat(outPath); err == nil {
		return Result{Name: name, Type: "envFile", Status: "skipped", Detail: f.Output + " already exists"}
	}

	tmplData, err := os.ReadFile(filepath.Join(dir, f.Template))
	if err != nil {
		return Result{Name: name, Type: "envFile", Status: "failed", Detail: "reading template: " + err.Error()}
	}

	values, err := resolveEnvVars(f.Vars, opts)
	if err != nil {
		return Result{Name: name, Type: "envFile", Status: "failed", Detail: err.Error()}
	}

	tmpl, err := template.New(f.Output).Option("missingkey=error").Parse(string(tmplData))
	if err != nil {
		return Result{Name: name, Type: "envFile", Status: "failed", Detail: "parsing template: " + err.Error()}
	}
	var rendered strings.Builder
	if err := tmpl.Execute(&rendered, values); err != nil {
		return Result{Name: name, Type: "envFile", Status: "failed", Detail: "rendering template: " + err.Error()}
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return Result{Name: name, Type: "envFile", Status: "failed", Detail: "creating output directory: " + err.Error()}
	}
	// 0600: these files commonly hold secrets.
	if err := os.WriteFile(outPath, []byte(rendered.String()), 0o600); err != nil {
		return Result{Name: name, Type: "envFile", Status: "failed", Detail: "writing output: " + err.Error()}
	}

	return Result{Name: name, Type: "envFile", Status: "written", Detail: f.Output}
}

// resolveEnvVars resolves each var's value, in priority order: an OS
// environment variable, then (when opts.Interactive) a terminal prompt,
// then Default. Returns an error naming the first var left unresolved.
func resolveEnvVars(vars []config.EnvVar, opts Options) (map[string]string, error) {
	values := make(map[string]string, len(vars))

	var reader *bufio.Reader
	if opts.Interactive {
		stdin := opts.Stdin
		if stdin == nil {
			stdin = os.Stdin
		}
		reader = bufio.NewReader(stdin)
	}
	stdout := opts.stepOut()

	for _, v := range vars {
		envVar := v.EnvVar
		if envVar == "" {
			envVar = v.Name
		}
		if val, ok := os.LookupEnv(envVar); ok && val != "" {
			values[v.Name] = val
			continue
		}

		value := v.Default
		if reader != nil {
			value = promptEnvVar(reader, stdout, v)
		}
		if value == "" {
			return nil, fmt.Errorf("%s: no value from env var %q, prompt, or default", v.Name, envVar)
		}
		values[v.Name] = value
	}
	return values, nil
}

// promptEnvVar prompts interactively for v's value, falling back to
// v.Default when the user enters a blank line.
func promptEnvVar(reader *bufio.Reader, stdout io.Writer, v config.EnvVar) string {
	label := v.Prompt
	if label == "" {
		label = v.Name
	}
	if v.Default != "" {
		fmt.Fprintf(stdout, "  %s [%s]: ", label, v.Default)
	} else {
		fmt.Fprintf(stdout, "  %s: ", label)
	}
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return v.Default
	}
	return line
}
