package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/dever-labs/devx/internal/task"
)

func runUp(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("up", flag.ContinueOnError)
	detach := fs.Bool("detach", false, "Start the stack and return immediately instead of attaching to logs")
	fs.BoolVar(detach, "d", false, "Shorthand for --detach")
	dryRun := fs.Bool("dry-run", false, "Print the resolved graph and commands, execute nothing")
	keepGoing := fs.Bool("keep-going", false, "Continue unrelated branches of the graph on failure")
	group := fs.String("group", "", "Only tasks whose group: matches (+ their deps)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	names, cliVars := parseCLIVars(fs.Args())

	m, err := loadValidManifest()
	if err != nil {
		return err
	}

	if err := ensureDevxDir(); err != nil {
		return err
	}

	sched, err := newScheduler(m, cliVars, task.Options{
		DryRun:    *dryRun,
		KeepGoing: *keepGoing,
		Group:     *group,
		Writer: func(name, msg string) {
			fmt.Printf("[%s] %s\n", name, msg)
		},
	})
	if err != nil {
		return err
	}

	if err := sched.Up(ctx, names); err != nil {
		return err
	}

	if !*dryRun {
		fmt.Println("\nEnvironment is up.")
	}

	if *detach || *dryRun {
		return nil
	}

	fmt.Println("Attached — streaming logs, Ctrl-C to stop the stack (devx down).")
	return attachAndWaitForInterrupt(ctx, sched)
}
