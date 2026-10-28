package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"

	"github.com/dever-labs/devx/internal/task"
)

func runLogs(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	follow := fs.Bool("follow", false, "Follow logs")
	if err := fs.Parse(args); err != nil {
		return err
	}

	m, err := loadValidManifest()
	if err != nil {
		return err
	}
	state, err := task.LoadState(stateFilePath())
	if err != nil {
		return err
	}

	if fs.NArg() > 0 {
		name := fs.Arg(0)
		if _, ok := m.Tasks[name]; !ok {
			return fmt.Errorf("unknown task %q", name)
		}
		entry, ok := state.Lookup(name)
		if !ok {
			return fmt.Errorf("task %q isn't currently running (no captured logs) — interactive: true tasks have nothing captured either, since output went straight to the terminal", name)
		}
		return tailOne(ctx, name, entry, *follow)
	}

	entries := state.All()
	if len(entries) == 0 {
		fmt.Println("No tasks currently running.")
		return nil
	}
	return tailAll(ctx, entries, *follow)
}

func tailOne(ctx context.Context, name string, entry task.StateEntry, follow bool) error {
	f, err := os.Open(entry.LogPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return streamPrefixed(ctx, name, f, follow, entry.LogPath)
}

func tailAll(ctx context.Context, entries map[string]task.StateEntry, follow bool) error {
	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	sort.Strings(names)

	// block forever on --follow; the process exits via Ctrl-C (SIGINT).
	for _, n := range names {
		n := n
		e := entries[n]
		go func() {
			f, err := os.Open(e.LogPath)
			if err != nil {
				return
			}
			defer f.Close()
			streamPrefixed(ctx, n, f, follow, e.LogPath) //nolint:errcheck
		}()
	}
	if follow {
		select {}
	}
	return nil
}

func streamPrefixed(ctx context.Context, label string, f *os.File, follow bool, path string) error {
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fmt.Printf("[%s] %s\n", label, scanner.Text())
	}
	if !follow {
		return scanner.Err()
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		for scanner.Scan() {
			fmt.Printf("[%s] %s\n", label, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		default:
		}
	}
}

// attachAndWaitForInterrupt streams every currently-running detached
// task's logs (devx up's default attached mode) until Ctrl-C, which then
// stops the stack — equivalent to running `devx down` afterward.
func attachAndWaitForInterrupt(ctx context.Context, sched *task.Scheduler) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	tailCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	entries := sched.State.All()
	for name, e := range entries {
		name, e := name, e
		go func() {
			f, err := os.Open(e.LogPath)
			if err != nil {
				return
			}
			defer f.Close()
			streamPrefixed(tailCtx, name, f, true, e.LogPath) //nolint:errcheck
		}()
	}

	<-sigCh
	fmt.Println("\nStopping stack (devx down)...")
	cancel()
	return runDown(context.Background(), nil)
}
