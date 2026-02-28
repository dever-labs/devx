package main

import (
	"context"
	"errors"
	"flag"
	"path/filepath"

	"github.com/dever-labs/devx/internal/runtime"
)

func runLogs(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	follow := fs.Bool("follow", false, "Follow logs")
	since := fs.String("since", "", "Show logs since")
	jsonOut := fs.Bool("json", false, "JSON output")
	_ = fs.Parse(args)

	var service string
	if fs.NArg() > 0 {
		service = fs.Arg(0)
	}

	manifest, profName, prof, err := loadProfile("")
	if err != nil {
		return err
	}

	if profileRuntime(prof) == "k8s" {
		return errors.New("logs for k8s runtime are not supported yet")
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

	reader, err := rt.Logs(ctx, composePath, manifest.Project.Name, runtime.LogsOptions{
		Service: service,
		Follow:  *follow,
		Since:   *since,
		JSON:    *jsonOut,
	})
	if err != nil {
		return err
	}
	defer reader.Close()

	return streamLogs(reader, *jsonOut)
}
