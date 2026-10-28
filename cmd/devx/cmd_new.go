package main

import (
	"flag"
	"fmt"
	"os"
)

// runNew implements `devx new <name> [--run <cmd>]`: appends a minimal
// task skeleton to devx.yaml. The old `devx new service|dep` scaffolding
// (Profile/Service/Dep-specific) has no equivalent in the task model —
// the bare-string shorthand (`name: ./some-command`) already makes
// scaffolding by hand nearly trivial, so this intentionally stays thin
// rather than reproducing the old interactive wizard.
func runNew(args []string) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	run := fs.String("run", "echo 'add your command here'", "The command this task runs")
	desc := fs.String("desc", "", "Optional one-line description")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: devx new <task-name> [--run <cmd>] [--desc <text>]")
	}
	name := fs.Arg(0)

	if !fileExists(manifestFile) {
		return fmt.Errorf("%s not found — run 'devx init' first", manifestFile)
	}

	var block string
	if *desc != "" {
		block = fmt.Sprintf("\n%s:\n  desc: %s\n  run: %s\n", name, *desc, *run)
	} else {
		block = fmt.Sprintf("\n%s: %s\n", name, *run)
	}

	f, err := os.OpenFile(manifestFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(block); err != nil {
		return err
	}

	fmt.Printf("✓ added task %q to %s\n", name, manifestFile)
	fmt.Printf("  run it with: devx run %s\n", name)
	return nil
}
