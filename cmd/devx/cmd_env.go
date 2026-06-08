package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func runEnv(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("env", flag.ExitOnError)
	showValues := fs.Bool("show", false, "Show actual values (secrets are masked by default)")
	_ = fs.Parse(args)

	// Collect env vars from devcontainer.json.
	dcVars := map[string]string{}
	if cfg, _, err := loadDevcontainerJSON(); err == nil {
		for k, v := range cfg.RemoteEnv {
			dcVars[k] = v
		}
		// ContainerEnv is also commonly set; it's already in RemoteEnv for most setups.
	}

	// Collect env vars from .env file.
	dotenvVars := map[string]string{}
	if data, err := os.ReadFile(".env"); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if k, v, ok := strings.Cut(line, "="); ok {
				dotenvVars[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}

	// Merge keys from both sources.
	allKeys := map[string]struct{}{}
	for k := range dcVars {
		allKeys[k] = struct{}{}
	}
	for k := range dotenvVars {
		allKeys[k] = struct{}{}
	}

	if len(allKeys) == 0 {
		fmt.Println("No environment variables defined in devcontainer.json or .env")
		return nil
	}

	keys := make([]string, 0, len(allKeys))
	for k := range allKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	missing := 0
	fmt.Printf("%-40s  %-12s  %s\n", "Variable", "Source", "Value")
	fmt.Println(strings.Repeat("─", 90))

	for _, k := range keys {
		source := sourceLabel(k, dcVars, dotenvVars)
		envVal := os.Getenv(k)
		dotVal := dotenvVars[k]

		// Effective value: actual env > .env file.
		effective := envVal
		if effective == "" {
			effective = dotVal
		}

		var display string
		switch {
		case effective == "":
			display = "⚠  not set"
			missing++
		case *showValues:
			display = effective
		case isLikelySecret(k):
			display = maskSecret(effective)
		default:
			display = effective
		}

		fmt.Printf("%-40s  %-12s  %s\n", k, source, display)
	}

	if missing > 0 {
		fmt.Println()
		fmt.Printf("⚠  %d variable(s) not set. Add them to .env or export them in your shell.\n", missing)
	}
	fmt.Println()
	fmt.Println("Tips:")
	fmt.Println("  devx env --show      reveal secret values")
	fmt.Println("  .env file            loaded automatically by  devx run  and devcontainer")
	return nil
}

func sourceLabel(k string, dcVars, dotenvVars map[string]string) string {
	_, inDC := dcVars[k]
	_, inDot := dotenvVars[k]
	switch {
	case inDC && inDot:
		return "dc + .env"
	case inDC:
		return "devcontainer"
	case inDot:
		return ".env"
	default:
		return "env"
	}
}

func isLikelySecret(key string) bool {
	lower := strings.ToLower(key)
	for _, word := range []string{"key", "secret", "token", "password", "passwd", "pwd", "credential", "auth"} {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

func maskSecret(val string) string {
	if len(val) <= 6 {
		return strings.Repeat("*", len(val))
	}
	return val[:3] + strings.Repeat("*", len(val)-6) + val[len(val)-3:]
}

// loadDotEnvToMap reads a .env file and returns k→v pairs.
// Used by execScriptOnHost to inject env vars before running scripts.
func loadDotEnvToMap(dotenvPath string) map[string]string {
	vars := map[string]string{}
	data, err := os.ReadFile(dotenvPath)
	if err != nil {
		return vars
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			k = strings.TrimSpace(k)
			v = strings.Trim(strings.TrimSpace(v), `"'`)
			vars[k] = v
		}
	}
	return vars
}

// dotEnvPath returns the path to the .env file in the given workdir.
func dotEnvPath(workdir string) string {
	if workdir != "" {
		abs, err := filepath.Abs(workdir)
		if err == nil {
			return filepath.Join(abs, ".env")
		}
	}
	return ".env"
}
