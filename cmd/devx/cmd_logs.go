package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"sync"

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

	var procNames []string
	if service != "" {
		if svc, ok := prof.Services[service]; ok && svc.IsProcess() {
			procNames = []string{service}
		}
	} else {
		for _, n := range sortedServiceNames(prof) {
			if prof.Services[n].IsProcess() {
				procNames = append(procNames, n)
			}
		}
	}

	wantContainers := prof.HasContainers() && (service == "" || len(procNames) == 0)
	if !wantContainers {
		return streamProcessLogs(ctx, procNames, *follow, *jsonOut)
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

	var wg sync.WaitGroup
	var procErr error
	if len(procNames) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			procErr = streamProcessLogs(ctx, procNames, *follow, *jsonOut)
		}()
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

	streamErr := streamLogs(reader, *jsonOut)
	wg.Wait()
	if streamErr != nil {
		return streamErr
	}
	return procErr
}

// streamProcessLogs prints process service logs, prefixing each line with the
// service name (or emitting {"service","line"} objects with --json).
func streamProcessLogs(ctx context.Context, names []string, follow, jsonOut bool) error {
	sup := supervisor()
	var mu sync.Mutex
	emit := func(name string) func(string) {
		return func(line string) {
			mu.Lock()
			defer mu.Unlock()
			if jsonOut {
				data, _ := json.Marshal(map[string]string{"service": name, "line": line})
				fmt.Println(string(data))
				return
			}
			fmt.Printf("%s | %s\n", name, line)
		}
	}

	var wg sync.WaitGroup
	errs := make([]error, len(names))
	for i, name := range names {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			errs[i] = sup.Follow(ctx, name, follow, emit(name))
		}(i, name)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
