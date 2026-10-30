package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/dever-labs/devx/internal/setup"
)

// runProjectSetup checks/installs required tools, runs any one-time setup
// steps, and scaffolds env files declared in devx.yaml. It's the second
// half of `devx init` — run directly whenever devx.yaml already existed,
// or right after the interactive wizard finishes creating a new one.
func runProjectSetup(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	fix := fs.Bool("fix", false, "Re-run runOnce steps and install missing tools")
	outputJSON := fs.Bool("json", false, "Emit results as JSON")
	noPrompt := fs.Bool("no-prompt", false, "Never prompt for envFiles values; fail fast if one can't be resolved from env/default")
	_ = fs.Parse(args)

	manifest, err := loadManifest()
	if err != nil {
		return err
	}

	opts := setup.Options{Fix: *fix}
	if *outputJSON {
		// Route subprocess stdout to stderr so JSON stdout stays clean.
		opts.Stdout = os.Stderr
	}
	opts.Interactive = !*outputJSON && !*noPrompt && stdinIsTerminal()
	allResults := []setup.Result{}

	if len(manifest.Tools) > 0 {
		if !*outputJSON {
			fmt.Println("Checking tools...")
		}
		results := setup.RunTools(ctx, manifest.Tools, opts)
		allResults = append(allResults, results...)
	}

	if len(manifest.Setup) > 0 {
		if !*outputJSON {
			fmt.Println("Running setup steps...")
		}
		results := setup.RunSteps(manifest.Setup, opts)
		allResults = append(allResults, results...)
	}

	if len(manifest.EnvFiles) > 0 {
		if !*outputJSON {
			fmt.Println("Scaffolding env files...")
		}
		results := setup.RunEnvFiles(manifest.Dir(), manifest.EnvFiles, opts)
		allResults = append(allResults, results...)
	}

	if *outputJSON {
		data, err := json.MarshalIndent(allResults, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if len(allResults) == 0 {
		fmt.Println("Nothing to do — no tools, setup steps, or envFiles declared in devx.yaml")
		return nil
	}

	failed := 0
	for _, r := range allResults {
		icon := setupStatusIcon(r.Status)
		line := fmt.Sprintf("  %s %-20s %s", icon, r.Name, r.Status)
		if r.Detail != "" {
			line += "  " + r.Detail
		}
		fmt.Println(line)
		if r.Status == "failed" || r.Status == "missing" {
			failed++
		}
	}

	fmt.Println()
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "%d item(s) failed or missing — run 'devx init --fix' to install\n", failed)
		return fmt.Errorf("setup incomplete")
	}
	fmt.Println("Dev environment is ready.")
	return nil
}

func setupStatusIcon(status string) string {
	switch status {
	case "ok", "installed", "written":
		return "✓"
	case "skipped", "not-applicable":
		return "–"
	case "failed", "missing":
		return "✗"
	default:
		return "?"
	}
}

// stdinIsTerminal reports whether stdin looks like an interactive terminal
// (as opposed to a pipe, redirect, or /dev/null) — used to decide whether
// prompting for envFiles values can succeed instead of blocking forever.
func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
