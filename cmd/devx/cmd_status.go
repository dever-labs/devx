package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"sort"

	"github.com/dever-labs/devx/internal/ui"
)

func runStatus(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	_ = fs.Parse(args)

	manifest, profName, prof, err := loadProfile("")
	if err != nil {
		return err
	}

	if profileRuntime(prof) == "k8s" {
		return errors.New("status for k8s runtime is not supported yet")
	}

	rt, err := selectRuntime(ctx)
	if err != nil {
		return err
	}

	enableTelemetry := telemetryFromState()
	composePath := filepath.Join(devxDir, composeFile)
	if err := ensureDevxDir(); err != nil {
		return err
	}
	if err := writeCompose(composePath, manifest, profName, prof, nil, enableTelemetry); err != nil {
		return err
	}

	statuses, err := rt.Status(ctx, composePath, manifest.Project.Name)
	if err != nil {
		return err
	}

	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Name < statuses[j].Name
	})

	headers := []string{"Service", "State", "Health", "Ports"}
	rows := make([][]string, 0, len(statuses))
	for _, st := range statuses {
		rows = append(rows, []string{st.Name, st.State, st.Health, st.Ports})
	}
	ui.PrintTable(os.Stdout, headers, rows)
	return nil
}
