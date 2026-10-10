package config

import (
	"path/filepath"
	"testing"
)

// Every shipped example manifest must parse and pass validation.
func TestExamplesValidate(t *testing.T) {
	paths, err := filepath.Glob("../../examples/*/devx.yaml")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no examples found: %v", err)
	}
	for _, p := range paths {
		m, err := Load(p)
		if err != nil {
			t.Errorf("%s: %v", p, err)
			continue
		}
		for name := range m.Profiles {
			if err := ValidateProfile(m, name); err != nil {
				t.Errorf("%s profile %s: %v", p, name, err)
			}
		}
		if err := ValidateTools(m); err != nil {
			t.Errorf("%s: %v", p, err)
		}
		if err := ValidateSetup(m); err != nil {
			t.Errorf("%s: %v", p, err)
		}
		if err := ValidateEnvFiles(m); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
}
