package config

import "testing"

func TestValidateTools_Valid(t *testing.T) {
	data := []byte(`version: 1
project:
  name: my-app
  defaultProfile: local
profiles:
  local:
    services:
      api:
        image: nginx:alpine
tools:
  - name: node
    version: "20"
    check: "node --version"
    install:
      windows: "winget install OpenJS.NodeJS.LTS"
      macos: "brew install node@20"
      linux: "apt-get install -y nodejs"
  - name: go
    check: "go version"
`)
	m, err := Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if err := ValidateTools(m); err != nil {
		t.Fatalf("expected no error for valid tools: %v", err)
	}
}

func TestValidateTools_MissingName(t *testing.T) {
	data := []byte(`version: 1
project:
  name: my-app
  defaultProfile: local
profiles:
  local:
    services:
      api:
        image: nginx:alpine
tools:
  - check: "node --version"
`)
	m, err := Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if err := ValidateTools(m); err == nil {
		t.Fatal("expected error for tool missing name")
	}
}

func TestValidateTools_MissingCheck(t *testing.T) {
	data := []byte(`version: 1
project:
  name: my-app
  defaultProfile: local
profiles:
  local:
    services:
      api:
        image: nginx:alpine
tools:
  - name: my-custom-tool
    version: "20"
`)
	m, err := Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if err := ValidateTools(m); err == nil {
		t.Fatal("expected error for tool missing check")
	}
}

func TestValidateSetup_Valid(t *testing.T) {
	data := []byte(`version: 1
project:
  name: my-app
  defaultProfile: local
profiles:
  local:
    services:
      api:
        image: nginx:alpine
setup:
  - name: restore
    run: "npm install"
    workdir: ./frontend
    runOnce: true
    platform: all
  - name: build-tools
    run: "go build ./..."
    platform: linux
`)
	m, err := Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if err := ValidateSetup(m); err != nil {
		t.Fatalf("expected no error for valid setup: %v", err)
	}
}

func TestValidateSetup_MissingName(t *testing.T) {
	data := []byte(`version: 1
project:
  name: my-app
  defaultProfile: local
profiles:
  local:
    services:
      api:
        image: nginx:alpine
setup:
  - run: "npm install"
`)
	m, err := Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if err := ValidateSetup(m); err == nil {
		t.Fatal("expected error for setup step missing name")
	}
}

func TestValidateSetup_MissingRun(t *testing.T) {
	data := []byte(`version: 1
project:
  name: my-app
  defaultProfile: local
profiles:
  local:
    services:
      api:
        image: nginx:alpine
setup:
  - name: restore
`)
	m, err := Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if err := ValidateSetup(m); err == nil {
		t.Fatal("expected error for setup step missing run")
	}
}

func TestValidateSetup_InvalidPlatform(t *testing.T) {
	data := []byte(`version: 1
project:
  name: my-app
  defaultProfile: local
profiles:
  local:
    services:
      api:
        image: nginx:alpine
setup:
  - name: restore
    run: "npm install"
    platform: freebsd
`)
	m, err := Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if err := ValidateSetup(m); err == nil {
		t.Fatal("expected error for invalid platform")
	}
}

func TestValidateToolsAndSetup_Empty(t *testing.T) {
	data := []byte(`version: 1
project:
  name: my-app
  defaultProfile: local
profiles:
  local:
    services:
      api:
        image: nginx:alpine
`)
	m, err := Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if err := ValidateTools(m); err != nil {
		t.Fatalf("expected no error for empty tools: %v", err)
	}
	if err := ValidateSetup(m); err != nil {
		t.Fatalf("expected no error for empty setup: %v", err)
	}
}

func TestValidateTools_WellKnownToolNeedsNoCheck(t *testing.T) {
	m := &Manifest{Tools: []Tool{{Name: "node"}}}
	if err := ValidateTools(m); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
