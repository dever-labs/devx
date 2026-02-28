package config

import "testing"

func TestValidateManifest(t *testing.T) {
    data := []byte(`version: 1
project:
  name: my-app
  defaultProfile: local
profiles:
  local:
    runtime: compose
    services:
      api:
        image: nginx:alpine
`)

    m, err := Parse(data)
    if err != nil {
        t.Fatalf("parse failed: %v", err)
    }

    if err := Validate(m); err != nil {
        t.Fatalf("validate failed: %v", err)
    }

    if err := ValidateProfile(m, "local"); err != nil {
        t.Fatalf("profile validation failed: %v", err)
    }
}

func TestValidateManifestErrors(t *testing.T) {
    data := []byte(`version: 2
project:
  name: ""
  defaultProfile: ""
profiles: {}
`)

    m, err := Parse(data)
    if err != nil {
        t.Fatalf("parse failed: %v", err)
    }

    if err := Validate(m); err == nil {
        t.Fatalf("expected validation error")
    }
}

func TestValidateProfileRuntime(t *testing.T) {
    data := []byte(`version: 1
project:
  name: my-app
  defaultProfile: local
profiles:
  local:
    runtime: bad
    services:
      api:
        image: nginx:alpine
`)

    m, err := Parse(data)
    if err != nil {
        t.Fatalf("parse failed: %v", err)
    }

    if err := ValidateProfile(m, "local"); err == nil {
        t.Fatalf("expected runtime validation error")
    }
}

func TestValidateProfileDependsOnMissing(t *testing.T) {
    data := []byte(`version: 1
project:
  name: my-app
  defaultProfile: local
profiles:
  local:
    runtime: compose
    services:
      api:
        image: nginx:alpine
        dependsOn:
          - db
`)

    m, err := Parse(data)
    if err != nil {
        t.Fatalf("parse failed: %v", err)
    }

    if err := ValidateProfile(m, "local"); err == nil {
        t.Fatalf("expected error for missing dependsOn target")
    }
}

func TestValidateProfileDepKind(t *testing.T) {
    data := []byte(`version: 1
project:
  name: my-app
  defaultProfile: local
profiles:
  local:
    runtime: compose
    services:
      api:
        image: nginx:alpine
    deps:
      cache:
        kind: memcached
`)

    m, err := Parse(data)
    if err != nil {
        t.Fatalf("parse failed: %v", err)
    }

    if err := ValidateProfile(m, "local"); err == nil {
        t.Fatalf("expected error for unsupported dep kind")
    }
}

func TestProfileByName(t *testing.T) {
    data := []byte(`version: 1
project:
  name: my-app
  defaultProfile: local
profiles:
  local:
    runtime: compose
    services:
      api:
        image: nginx:alpine
`)

    m, err := Parse(data)
    if err != nil {
        t.Fatalf("parse failed: %v", err)
    }

    prof, err := ProfileByName(m, "local")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if prof.Runtime != "compose" {
        t.Fatalf("expected runtime compose, got %q", prof.Runtime)
    }

    if _, err := ProfileByName(m, "missing"); err == nil {
        t.Fatalf("expected error for missing profile")
    }
}

