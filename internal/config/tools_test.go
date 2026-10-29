package config

import "testing"

func TestValidateTools_Valid(t *testing.T) {
	data := []byte(`version: 1
project:
  name: my-app
api: ./bin/api
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
api: ./bin/api
tools:
  - check: "node --version"
`)
	m, err := Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if err := ValidateTools(m); err == nil {
		t.Fatalf("expected error for missing tool name")
	}
}

func TestValidateTools_MissingCheck(t *testing.T) {
	data := []byte(`version: 1
project:
  name: my-app
api: ./bin/api
tools:
  - name: node
`)
	m, err := Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if err := ValidateTools(m); err == nil {
		t.Fatalf("expected error for missing check")
	}
}

func TestValidateSetup_Valid(t *testing.T) {
	data := []byte(`version: 1
project:
  name: my-app
api: ./bin/api
setup:
  - name: install-deps
    run: "npm install"
    platform: all
`)
	m, err := Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if err := ValidateSetup(m); err != nil {
		t.Fatalf("expected no error for valid setup: %v", err)
	}
}

func TestValidateSetup_BadPlatform(t *testing.T) {
	data := []byte(`version: 1
project:
  name: my-app
api: ./bin/api
setup:
  - name: install-deps
    run: "npm install"
    platform: solaris
`)
	m, err := Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if err := ValidateSetup(m); err == nil {
		t.Fatalf("expected error for bad platform")
	}
}

func TestValidateSetup_MissingName(t *testing.T) {
	data := []byte(`version: 1
project:
  name: my-app
api: ./bin/api
setup:
  - run: "npm install"
`)
	m, err := Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if err := ValidateSetup(m); err == nil {
		t.Fatalf("expected error for missing setup step name")
	}
}

func TestValidateSetup_MissingRun(t *testing.T) {
	data := []byte(`version: 1
project:
  name: my-app
api: ./bin/api
setup:
  - name: install-deps
`)
	m, err := Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if err := ValidateSetup(m); err == nil {
		t.Fatalf("expected error for missing setup step run")
	}
}
