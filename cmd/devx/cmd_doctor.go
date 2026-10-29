package main

import (
	"context"
	"errors"
	"flag"
	"os"

	"github.com/dever-labs/devx/internal/doctor"
)

func runDoctor(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	fix := fs.Bool("fix", false, "Attempt to install missing tools and fix issues")
	outputJSON := fs.Bool("json", false, "Emit report as JSON")
	_ = fs.Parse(args)

	manifest, _ := loadManifest()

	report := doctor.Run(ctx, doctor.Options{
		Manifest: manifest,
		Fix:      *fix,
	})

	doctor.PrintReport(os.Stdout, report, *outputJSON)
	if report.HasFailures() {
		return errors.New("doctor found failures")
	}
	return nil
}

