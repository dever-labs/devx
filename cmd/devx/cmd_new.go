package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// runNew implements `devx new <name> [--run <cmd>]`: appends a minimal
// task skeleton to devx.yaml. The old `devx new service|dep` scaffolding
// (Profile/Service/Dep-specific) has no equivalent in the task model —
// the bare-string shorthand (`name: ./some-command`) already makes
// scaffolding by hand nearly trivial, so this intentionally stays thin
// rather than reproducing the old interactive wizard.
func runNew(args []string) error {
	// flag.Parse stops at the first non-flag argument, so the documented
	// usage `devx new <name> --run ... --desc ...` (name before its flags)
	// would otherwise fail to parse --run/--desc at all. Pull the bare
	// task-name positional out ourselves so it can appear before, after,
	// or between flags.
	name, flagArgs, err := extractPositional(args, "run", "desc")
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	run := fs.String("run", "echo 'add your command here'", "The command this task runs")
	desc := fs.String("desc", "", "Optional one-line description")
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if name == "" || fs.NArg() != 0 {
		return fmt.Errorf("usage: devx new <task-name> [--run <cmd>] [--desc <text>]")
	}

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

// extractPositional pulls the single bare (non-flag) positional argument
// out of args, returning it along with the remaining flag tokens (still
// in order, suitable for flag.FlagSet.Parse). valueFlags lists the names
// of string flags that consume the following argument as their value
// (so that value isn't mistaken for the positional). Returns an error if
// more than one bare positional is found.
func extractPositional(args []string, valueFlags ...string) (string, []string, error) {
	takesValue := map[string]bool{}
	for _, f := range valueFlags {
		takesValue[f] = true
	}

	var name string
	var flagArgs []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
			trimmed := strings.TrimLeft(a, "-")
			if !strings.Contains(trimmed, "=") && takesValue[trimmed] && i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
			continue
		}
		if name != "" {
			return "", nil, fmt.Errorf("usage: devx new <task-name> [--run <cmd>] [--desc <text>]")
		}
		name = a
	}
	return name, flagArgs, nil
}
