package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/dever-labs/devx/internal/task"
)

func runRun(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "Print the resolved command, execute nothing")
	watch := fs.Bool("watch", false, "Re-run whenever the task's sources: globs change")
	positional, err := parseFlagsAnywhere(fs, args)
	if err != nil {
		return err
	}

	names, cliVars := parseCLIVars(positional)
	if len(names) == 0 {
		return listTasks()
	}

	m, err := loadValidManifest()
	if err != nil {
		return err
	}
	for _, n := range names {
		if _, ok := m.Tasks[n]; !ok {
			return fmt.Errorf("unknown task %q — run 'devx tasks' to list available tasks", n)
		}
	}

	if *watch {
		if len(names) != 1 {
			return fmt.Errorf("--watch supports exactly one task at a time")
		}
		if len(m.Tasks[names[0]].Sources) == 0 {
			return fmt.Errorf("task %q has no sources: to watch", names[0])
		}
	}

	if err := ensureDevxDir(); err != nil {
		return err
	}

	sched, err := newScheduler(m, cliVars, task.Options{
		DryRun: *dryRun,
		Writer: func(name, msg string) { fmt.Printf("[%s] %s\n", name, msg) },
	})
	if err != nil {
		return err
	}

	if *watch {
		return watchAndRerun(ctx, sched, m, names[0])
	}

	return sched.Run(ctx, names)
}

func listTasks() error {
	m, err := loadValidManifest()
	if err != nil {
		return err
	}
	if len(m.Tasks) == 0 {
		fmt.Println("No tasks defined in devx.yaml.")
		return nil
	}
	fmt.Println("Available tasks:")
	fmt.Println()
	maxLen := 0
	for name := range m.Tasks {
		if len(name) > maxLen {
			maxLen = len(name)
		}
	}
	for name, t := range m.Tasks {
		desc := t.Desc
		if desc == "" {
			desc = t.Run
			if len(desc) > 55 {
				desc = desc[:55] + "…"
			}
		}
		fmt.Printf("  devx run %-*s  %s\n", maxLen, name, desc)
	}
	return nil
}
