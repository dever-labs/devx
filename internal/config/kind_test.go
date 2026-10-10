package config

import (
	"strings"
	"testing"
)

func validateYAML(t *testing.T, y string) error {
	t.Helper()
	m, err := Parse([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	return ValidateProfile(m, "local")
}

func TestProcessServiceValid(t *testing.T) {
	err := validateYAML(t, `
version: 1
project: {name: x, defaultProfile: local}
profiles:
  local:
    services:
      web:
        kind: process
        run: yarn dev
        ports: ["3000"]
        ready: {http: "http://localhost:3000", timeout: 30s}
        debug: {port: 9229, type: node}
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestProcessServiceInvalid(t *testing.T) {
	cases := map[string]string{
		"no command":       "kind: process",
		"image":            "kind: process\n        run: x\n        image: foo",
		"both":             "kind: process\n        run: x\n        command: [a]",
		"bad kind":         "kind: vm\n        image: foo",
		"run on container": "image: foo\n        run: x",
		"bad timeout":      "image: foo\n        ready: {http: http://x, timeout: soon}",
		"empty ready":      "image: foo\n        ready: {interval: 1s}",
		"bad debug port":   "image: foo\n        debug: {port: 0}",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			err := validateYAML(t, "version: 1\nproject: {name: x, defaultProfile: local}\nprofiles:\n  local:\n    services:\n      web:\n        "+body+"\n")
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), "web") {
				t.Fatalf("error should name service: %v", err)
			}
		})
	}
}

func TestProcessServiceRejectedOnK8s(t *testing.T) {
	err := validateYAML(t, "version: 1\nproject: {name: x, defaultProfile: local}\nprofiles:\n  local:\n    runtime: k8s\n    services:\n      web:\n        kind: process\n        run: x\n")
	if err == nil || !strings.Contains(err.Error(), "k8s") {
		t.Fatalf("expected k8s rejection, got %v", err)
	}
}
