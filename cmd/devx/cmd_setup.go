package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/dever-labs/devx/internal/setup"
)

func runSetup(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	fix := fs.Bool("fix", false, "Re-run runOnce steps, install missing tools, regenerate env files")
	outputJSON := fs.Bool("json", false, "Emit results as JSON")
	nonInteractive := fs.Bool("non-interactive", false, "Never prompt; fail listing missing values")
	_ = fs.Parse(args)

	manifest, err := loadManifestOnly()
	if err != nil {
		return err
	}

	opts := setup.Options{Fix: *fix}
	if *outputJSON {
		// Route subprocess stdout to stderr so JSON stdout stays clean.
		opts.Stdout = os.Stderr
	}
	allResults := []setup.Result{}

	if len(manifest.Tools) > 0 {
		if !*outputJSON {
			fmt.Println("Checking tools...")
		}
		results := setup.RunTools(ctx, manifest.Tools, opts)
		allResults = append(allResults, results...)
	}

	if len(manifest.EnvFiles) > 0 {
		if !*outputJSON {
			fmt.Println("Generating local config files...")
		}
		interactive := !*outputJSON && !*nonInteractive && os.Getenv("CI") == "" && stdinIsTerminal()
		allResults = append(allResults, setup.RunEnvFiles(manifest.EnvFiles, setup.EnvOptions{Fix: *fix, Interactive: interactive})...)
	}

	wd, _ := os.Getwd()
	if steps := setup.ResolveSteps(wd, manifest.Setup); len(steps) > 0 {
		if !*outputJSON {
			fmt.Println("Running setup steps...")
		}
		results := setup.RunSteps(steps, opts)
		allResults = append(allResults, results...)
	}

	if *outputJSON {
		data, err := json.MarshalIndent(allResults, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		for _, r := range allResults {
			if r.Status == "failed" || r.Status == "missing" {
				return fmt.Errorf("setup incomplete")
			}
		}
		return nil
	}

	if len(allResults) == 0 {
		fmt.Println("Nothing to do — no tools or setup steps declared in devx.yaml")
		return nil
	}

	failed := 0
	for _, r := range allResults {
		icon := setupStatusIcon(r.Status)
		line := fmt.Sprintf("  %s %-20s %s", icon, r.Name, r.Status)
		if r.Detail != "" {
			line += "  " + r.Detail
		}
		if r.Source != "" {
			line += "  (from " + r.Source + ")"
		}
		if r.Warning != "" {
			line += "\n      ! " + r.Warning
		}
		fmt.Println(line)
		if r.Status == "failed" || r.Status == "missing" {
			failed++
		}
	}

	fmt.Println()
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "%d item(s) failed or missing — run 'devx setup --fix' to install\n", failed)
		return fmt.Errorf("setup incomplete")
	}
	fmt.Println("Dev environment is ready.")
	return nil
}

func setupStatusIcon(status string) string {
	switch status {
	case "ok", "installed":
		return "✓"
	case "skipped", "not-applicable":
		return "–"
	case "failed", "missing":
		return "✗"
	default:
		return "?"
	}
}

func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
