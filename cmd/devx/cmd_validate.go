package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/dever-labs/devx/internal/config"
)

func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	file := fs.String("file", manifestFile, "Path to devx.yaml to validate")
	outputJSON := fs.Bool("json", false, "Emit result as JSON")
	_ = fs.Parse(args)

	manifest, err := config.Load(*file)
	if err != nil {
		if *outputJSON {
			if jsonErr := printValidateJSON(false, []string{fmt.Sprintf("parse error: %v", err)}); jsonErr != nil {
				return jsonErr
			}
			return fmt.Errorf("validation failed")
		}
		fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
		return fmt.Errorf("validation failed")
	}

	var issues []string

	if err := config.Validate(manifest); err != nil {
		if ve, ok := err.(*config.ValidationError); ok {
			issues = append(issues, ve.Issues...)
		} else {
			issues = append(issues, err.Error())
		}
	}

	for profName := range manifest.Profiles {
		if err := config.ValidateProfile(manifest, profName); err != nil {
			if ve, ok := err.(*config.ValidationError); ok {
				for _, issue := range ve.Issues {
					issues = append(issues, fmt.Sprintf("[profile:%s] %s", profName, issue))
				}
			} else {
				issues = append(issues, fmt.Sprintf("[profile:%s] %s", profName, err.Error()))
			}
		}
	}

	if err := config.ValidateTools(manifest); err != nil {
		if ve, ok := err.(*config.ValidationError); ok {
			issues = append(issues, ve.Issues...)
		} else {
			issues = append(issues, err.Error())
		}
	}

	if err := config.ValidateSetup(manifest); err != nil {
		if ve, ok := err.(*config.ValidationError); ok {
			issues = append(issues, ve.Issues...)
		} else {
			issues = append(issues, err.Error())
		}
	}

	if err := config.ValidateEnvFiles(manifest); err != nil {
		if ve, ok := err.(*config.ValidationError); ok {
			issues = append(issues, ve.Issues...)
		} else {
			issues = append(issues, err.Error())
		}
	}

	if *outputJSON {
		if err := printValidateJSON(len(issues) == 0, issues); err != nil {
			return err
		}
		if len(issues) > 0 {
			return fmt.Errorf("validation failed")
		}
		return nil
	}

	if len(issues) > 0 {
		fmt.Fprintf(os.Stderr, "devx.yaml has %d issue(s):\n", len(issues))
		for _, issue := range issues {
			fmt.Fprintf(os.Stderr, "  - %s\n", issue)
		}
		return fmt.Errorf("validation failed")
	}

	fmt.Println("devx.yaml is valid ✓")
	return nil
}

func printValidateJSON(valid bool, issues []string) error {
	out := struct {
		Valid  bool     `json:"valid"`
		Issues []string `json:"issues"`
	}{Valid: valid, Issues: issues}
	if out.Issues == nil {
		out.Issues = []string{}
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
