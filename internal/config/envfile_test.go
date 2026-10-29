package config

import "testing"

func TestEnvFiles_ParseAndValidate_Valid(t *testing.T) {
	data := []byte(`version: 1
project:
  name: my-app
envFiles:
  - name: local app config
    template: app-config.local.yaml.tmpl
    output: app-config.local.yaml
    vars:
      - name: DB_PASSWORD
        envVar: DB_PASSWORD
        prompt: "Database password"
        secret: true
      - name: API_URL
        default: "http://localhost:8080"
`)
	m, err := Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(m.EnvFiles) != 1 {
		t.Fatalf("expected 1 envFiles entry, got %d", len(m.EnvFiles))
	}
	f := m.EnvFiles[0]
	if f.Template != "app-config.local.yaml.tmpl" || f.Output != "app-config.local.yaml" {
		t.Errorf("unexpected envFiles[0] = %+v", f)
	}
	if len(f.Vars) != 2 {
		t.Fatalf("expected 2 vars, got %d", len(f.Vars))
	}
	if !f.Vars[0].Secret {
		t.Error("expected DB_PASSWORD var to be marked secret")
	}
	if err := ValidateEnvFiles(m); err != nil {
		t.Fatalf("expected no error for valid envFiles: %v", err)
	}
}

func TestValidateEnvFiles_MissingTemplate(t *testing.T) {
	m := &Manifest{EnvFiles: []EnvFile{{Output: "out.yaml"}}}
	if err := ValidateEnvFiles(m); err == nil {
		t.Fatal("expected an error for missing template")
	}
}

func TestValidateEnvFiles_MissingOutput(t *testing.T) {
	m := &Manifest{EnvFiles: []EnvFile{{Template: "tmpl.yaml"}}}
	if err := ValidateEnvFiles(m); err == nil {
		t.Fatal("expected an error for missing output")
	}
}

func TestValidateEnvFiles_MissingVarName(t *testing.T) {
	m := &Manifest{EnvFiles: []EnvFile{{
		Template: "tmpl.yaml",
		Output:   "out.yaml",
		Vars:     []EnvVar{{Default: "x"}},
	}}}
	if err := ValidateEnvFiles(m); err == nil {
		t.Fatal("expected an error for a var with no name")
	}
}
