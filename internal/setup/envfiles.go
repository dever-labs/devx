package setup

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dever-labs/devx/internal/config"
)

// EnvOptions controls env file generation.
type EnvOptions struct {
	Fix         bool // overwrite existing files
	Interactive bool // allow prompt sources to ask the user
}

var placeholderRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// RunEnvFiles generates each declared env file. Existing files are left alone
// unless opts.Fix is set. Values are never included in results or logs; on
// failure, Detail lists only the keys that could not be resolved.
func RunEnvFiles(files []config.EnvFile, opts EnvOptions) []Result {
	var results []Result
	for _, f := range files {
		results = append(results, runEnvFile(f, opts))
	}
	return results
}

func runEnvFile(f config.EnvFile, opts EnvOptions) Result {
	res := func(status, detail string) Result {
		return Result{Name: f.File, Type: "env", Status: status, Detail: detail}
	}
	if _, err := os.Stat(f.File); err == nil && !opts.Fix {
		return res("skipped", "already exists (use --fix to regenerate)")
	}

	keys := make([]string, 0, len(f.Values))
	for k := range f.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	vals := map[string]string{}
	var missing []string
	var failed []string
	in := bufio.NewReader(os.Stdin)
	for _, k := range keys {
		v, status := resolveValue(k, f.Values[k], opts, in)
		switch status {
		case "missing":
			missing = append(missing, k)
		case "failed":
			failed = append(failed, k)
		default:
			vals[k] = v
		}
	}
	if len(failed) > 0 {
		return res("failed", "command failed for: "+strings.Join(failed, ", "))
	}
	if len(missing) > 0 {
		return res("missing", "no value for: "+strings.Join(missing, ", ")+
			" (set the env var, add a default, or run interactively)")
	}

	body, err := renderEnvFile(f, keys, vals)
	if err != nil {
		return res("failed", err.Error())
	}
	if dir := filepath.Dir(f.File); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return res("failed", err.Error())
		}
	}
	if err := os.WriteFile(f.File, []byte(body), 0o600); err != nil {
		return res("failed", err.Error())
	}
	detail := fmt.Sprintf("wrote %d value(s)", len(keys))
	if !gitIgnored(f.File) {
		detail += "; warning: not gitignored — add it to .gitignore"
	}
	return res("ok", detail)
}

// resolveValue returns the value and a status of "" (ok), "missing" or "failed".
func resolveValue(key string, s config.EnvSource, opts EnvOptions, in *bufio.Reader) (string, string) {
	fallback := func() (string, string) {
		if s.Default == "generate" {
			b := make([]byte, 16)
			if _, err := rand.Read(b); err != nil {
				return "", "failed"
			}
			return hex.EncodeToString(b), ""
		}
		if s.Default != "" {
			return s.Default, ""
		}
		return "", "missing"
	}
	switch s.From {
	case "env":
		name := s.Name
		if name == "" {
			name = key
		}
		if v, ok := os.LookupEnv(name); ok && v != "" {
			return v, ""
		}
		return fallback()
	case "command":
		cmd := shellCmd(s.Run)
		cmd.Stderr = nil
		out, err := cmd.Output()
		if err != nil || strings.TrimSpace(string(out)) == "" {
			if s.Default != "" {
				return fallback()
			}
			return "", "failed"
		}
		return strings.TrimRight(string(out), "\r\n"), ""
	case "prompt":
		if !opts.Interactive {
			return fallback()
		}
		if s.Default == "generate" {
			return fallback()
		}
		fmt.Fprintf(os.Stderr, "  %s", key)
		if s.Default != "" {
			fmt.Fprintf(os.Stderr, " [%s]", s.Default)
		}
		fmt.Fprint(os.Stderr, ": ")
		if s.Secret {
			setEcho(false)
			defer func() { setEcho(true); fmt.Fprintln(os.Stderr) }()
		}
		line, _ := in.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			return fallback()
		}
		return line, ""
	}
	return "", "missing"
}

func renderEnvFile(f config.EnvFile, keys []string, vals map[string]string) (string, error) {
	if f.Template == "" {
		var b strings.Builder
		for _, k := range keys {
			fmt.Fprintf(&b, "%s=%s\n", k, vals[k])
		}
		return b.String(), nil
	}
	raw, err := os.ReadFile(f.Template)
	if err != nil {
		return "", fmt.Errorf("reading template: %w", err)
	}
	var unknown []string
	out := placeholderRe.ReplaceAllStringFunc(string(raw), func(m string) string {
		k := placeholderRe.FindStringSubmatch(m)[1]
		v, ok := vals[k]
		if !ok {
			unknown = append(unknown, k)
			return m
		}
		return v
	})
	if len(unknown) > 0 {
		return "", fmt.Errorf("template references undeclared keys: %s", strings.Join(unknown, ", "))
	}
	return out, nil
}

func gitIgnored(path string) bool {
	if _, err := exec.LookPath("git"); err != nil {
		return true
	}
	return exec.Command("git", "check-ignore", "-q", path).Run() == nil
}
