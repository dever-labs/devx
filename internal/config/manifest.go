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
	AI       *AIConfig          `yaml:"ai,omitempty"`
	Profiles map[string]Profile `yaml:"profiles"`
	// Tools declares required SDKs, runtimes, and CLI tools for the project.
	// Use `devx doctor` to check and `devx setup` (or `devx doctor --fix`) to install.
	Tools []Tool `yaml:"tools,omitempty"`
	// Setup declares ordered host-side commands to run after tool installation.
	// Use `devx setup` to execute. RunOnce steps are skipped when unchanged.
	Setup []SetupStep `yaml:"setup,omitempty"`
	// Scripts declares named shell commands runnable with `devx run <name>`.
	Scripts []Script `yaml:"scripts,omitempty"`
}

// Script is a named, runnable shell command declared in devx.yaml.
// Run with: devx run <name>
type Script struct {
	Name      string `yaml:"name"`
	Run       string `yaml:"run"`
	Desc      string `yaml:"desc,omitempty"`
	Workdir   string `yaml:"workdir,omitempty"`
	// Container controls where the script runs when a devcontainer is present.
	//   auto  (default) — run inside the devcontainer if it is running, host otherwise
	//   true             — always run inside the devcontainer (fail if not running)
	//   false            — always run on the host
	Container string `yaml:"container,omitempty"` // auto | true | false
}

// AIConfig holds optional AI provider settings used by 'devx export',
// automatic connection string detection, and local inference setup.
// Credentials are read from environment variables:
//
//	openai:       OPENAI_API_KEY
//	anthropic:    ANTHROPIC_API_KEY
//	azure-openai: AZURE_OPENAI_KEY
//	ollama/local: no auth required
type AIConfig struct {
	Provider string        `yaml:"provider"` // openai | anthropic | ollama | azure-openai | local
	Model    string        `yaml:"model"`
	BaseURL  string        `yaml:"baseURL,omitempty"` // override endpoint
	Local    *LocalAIConfig `yaml:"local,omitempty"`
}

// LocalAIConfig configures the local inference backend used by 'devx ai setup/status'.
type LocalAIConfig struct {
	// Backend selects the inference engine: mlx | ollama | auto (default).
	// auto picks MLX on Apple Silicon and Ollama elsewhere.
	Backend string `yaml:"backend,omitempty"`
	// Model overrides the main chat/reasoning model for local inference.
	// Defaults to AIConfig.Model.
	Model string `yaml:"model,omitempty"`
	// AutocompleteModel is the fast model used for tab completion.
	// Defaults to qwen2.5-coder:14b.
	AutocompleteModel string `yaml:"autocompleteModel,omitempty"`
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
//	      Set background: true to start the process without waiting for it to exit.
//	      devx up will stream its output (prefixed with name) and block until it stops.
//	      Use name to label output lines; defaults to the run command.
type Hook struct {
	// Exec is the command to run inside Service (e.g. "migrate up").
	Exec    string `yaml:"exec"`
	Service string `yaml:"service"`
	// Run is a host-side shell command (e.g. "./scripts/seed.sh").
	Run        string `yaml:"run"`
	Background bool   `yaml:"background,omitempty"`
	Name       string `yaml:"name,omitempty"`
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

// Dep is a third-party dependency (database, cache, broker, …) that devx
// runs as a container. The project fully controls which image to run via
// Image.
//
// When Kind is set, devx downloads a provider plugin that contributes
// behavioural logic (health checks, compose fragments, connection string
// templates). Source defaults to "devx-labs/<kind>" if omitted. Version
// follows the major-version convention: major = dep major version
// (e.g. "16.1.0" = provider for PostgreSQL 16, provider patch release 1.0).
//
// The Connect block lists services that should have connection environment
// variables injected automatically. Each entry can supply an explicit Env
// mapping using ${host}, ${port}, or any dep env key as template variables.
// If Env is omitted and AI is configured in the manifest, devx scans the
// service source directory and uses the LLM to detect the correct env var names.
type Dep struct {
	Kind    string `yaml:"kind,omitempty"`
	Source  string `yaml:"source,omitempty"`
	Version string `yaml:"version,omitempty"`
	Image   string `yaml:"image,omitempty"`

	Env     map[string]string `yaml:"env"`
	Ports   []string          `yaml:"ports"`
	Volume  string            `yaml:"volume"`
	Connect []ConnectEntry    `yaml:"connect,omitempty"`
}

// ConnectEntry declares a service that a dep should inject connection
// environment variables into. Env values support template variables:
//   - ${host}   — the dep's service name within the compose network
//   - ${port}   — the first container-side port declared in dep.ports
//   - ${<KEY>}  — any key from the dep's own env block (e.g. ${POSTGRES_PASSWORD})
//
// If Env is omitted and devx.yaml has an ai block, devx calls the LLM to
// detect appropriate env var names by scanning the service's build context.
type ConnectEntry struct {
	Service string            `yaml:"service"`
	Env     map[string]string `yaml:"env,omitempty"`
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
