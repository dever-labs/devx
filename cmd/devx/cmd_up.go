package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/dever-labs/devx/internal/config"
	"github.com/dever-labs/devx/internal/setup"
	"github.com/dever-labs/devx/internal/task"
)

func runUp(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("up", flag.ContinueOnError)
	detach := fs.Bool("detach", false, "Start the stack and return immediately instead of attaching to logs")
	fs.BoolVar(detach, "d", false, "Shorthand for --detach")
	dryRun := fs.Bool("dry-run", false, "Print the resolved graph and commands, execute nothing")
	keepGoing := fs.Bool("keep-going", false, "Continue unrelated branches of the graph on failure")
	group := fs.String("group", "", "Only tasks whose group: matches (+ their deps)")
	positional, err := parseFlagsAnywhere(fs, args)
	if err != nil {
		return err
	}

	names, cliVars := parseCLIVars(positional)

	m, err := loadValidManifest()
	if err != nil {
		return err
	}

	if err := ensureDevxDir(); err != nil {
		return err
	}

	if !*dryRun {
		runEnvFilesBestEffort(m)
	}

	if len(names) == 0 && *group == "" && len(m.Default) > 0 {
		names = expandDefaultTargets(m)
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

// expandDefaultTargets resolves manifest.Default entries (each either a
// task name or a group: name) to the set of root task names bare
// `devx up` should run (see issue #55). Validate already confirms every
// entry matches a task or a group, so this never returns empty for a
// non-empty Default.
func expandDefaultTargets(m *config.Manifest) []string {
	var names []string
	for _, d := range m.Default {
		if _, ok := m.Tasks[d]; ok {
			names = append(names, d)
			continue
		}
		for taskName, t := range m.Tasks {
			if t.Group == d {
				names = append(names, taskName)
			}
		}
	}
	return names
}

// runEnvFilesBestEffort scaffolds/merges envFiles: as an implicit,
// dependency-free step of `devx up` (see issue #61) so a newly required
// key reaching an existing .env doesn't require re-running `devx init`
// by hand. Never prompts (devx up shouldn't block on stdin) and never
// fails devx up — a problem here is reported but left for `devx init`
// (which does fail loudly) to resolve.
func runEnvFilesBestEffort(m *config.Manifest) {
	if len(m.EnvFiles) == 0 {
		return
	}
	results := setup.RunEnvFiles(m.Dir(), m.EnvFiles, setup.Options{Interactive: false})
	for _, r := range results {
		switch r.Status {
		case "merged", "written":
			fmt.Printf("[envFiles] %s: %s\n", r.Name, r.Detail)
		case "failed":
			fmt.Fprintf(os.Stderr, "[envFiles] %s: %s (run 'devx init' to fix)\n", r.Name, r.Detail)
		}
	}
}
