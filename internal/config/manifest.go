package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Manifest struct {
	Version  int                `yaml:"version"`
	Project  Project            `yaml:"project"`
	Registry Registry           `yaml:"registry"`
	Profiles map[string]Profile `yaml:"profiles"`
}

type Project struct {
	Name           string `yaml:"name"`
	DefaultProfile string `yaml:"defaultProfile"`
}

type Registry struct {
	Prefix string `yaml:"prefix"`
}

type Profile struct {
	Services map[string]Service `yaml:"services"`
	Deps     map[string]Dep     `yaml:"deps"`
	Runtime  string             `yaml:"runtime"`
	Hooks    Hooks              `yaml:"hooks"`
}

// Hooks defines commands to run at lifecycle points around devx up/down.
type Hooks struct {
	AfterUp    []Hook `yaml:"afterUp"`
	BeforeDown []Hook `yaml:"beforeDown"`
}

// Hook is a single lifecycle step. Exactly one of Exec or Run must be set.
//
//	exec: runs a command inside an already-running container via `docker compose exec`.
//	      Service is required.
//	run:  runs a command on the host via the system shell.
type Hook struct {
	// Exec is the command to run inside Service (e.g. "migrate up").
	Exec    string `yaml:"exec"`
	Service string `yaml:"service"`
	// Run is a host-side shell command (e.g. "./scripts/seed.sh").
	Run string `yaml:"run"`
}

type Service struct {
	Image     string            `yaml:"image"`
	Build     *Build            `yaml:"build"`
	Ports     []string          `yaml:"ports"`
	Env       map[string]string `yaml:"env"`
	Command   []string          `yaml:"command"`
	Workdir   string            `yaml:"workdir"`
	Mount     []string          `yaml:"mount"`
	DependsOn []string          `yaml:"dependsOn"`
	Health    *Health           `yaml:"health"`
}

type Build struct {
	Context    string `yaml:"context"`
	Dockerfile string `yaml:"dockerfile"`
}

type Health struct {
	HttpGet  string `yaml:"httpGet"`
	Interval string `yaml:"interval"`
	Retries  int    `yaml:"retries"`
}

type Dep struct {
	Kind    string            `yaml:"kind"`
	Version string            `yaml:"version"`
	Env     map[string]string `yaml:"env"`
	Ports   []string          `yaml:"ports"`
	Volume  string            `yaml:"volume"`
}

func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}
	return &m, nil
}

func ProfileByName(m *Manifest, name string) (*Profile, error) {
	prof, ok := m.Profiles[name]
	if !ok {
		return nil, fmt.Errorf("profile '%s' not found", name)
	}
	return &prof, nil
}
