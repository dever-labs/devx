package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// reservedTopLevelKeys are the only top-level devx.yaml keys that are not
// task names. Everything else is a Task.
var reservedTopLevelKeys = map[string]bool{
	"version":  true,
	"project":  true,
	"ai":       true,
	"tools":    true,
	"setup":    true,
	"vars":     true,
	"includes": true,
	"agents":   true,
}

// Manifest is the parsed, not-yet-merged shape of a single devx.yaml file.
// Task names are top-level keys directly — no tasks:/services:/profiles:
// wrapper. Use Load to get a fully merged Manifest with includes: resolved
// and task names namespaced.
type Manifest struct {
	Version int       `yaml:"version"`
	Project Project   `yaml:"project"`
	AI      *AIConfig `yaml:"ai,omitempty"`
	// Tools declares required SDKs, runtimes, and CLI tools for the project.
	// Use `devx doctor` to check and `devx setup` (or `devx doctor --fix`) to install.
	Tools []Tool `yaml:"tools,omitempty"`
	// Setup declares ordered host-side commands to run after tool installation.
	// Use `devx setup` to execute. RunOnce steps are skipped when unchanged.
	Setup []SetupStep `yaml:"setup,omitempty"`
	// Vars are plain textual ${VAR} substitution defaults, lowest-priority
	// in the CLI override > inherited env > .env/.env.local > vars: chain.
	Vars map[string]string `yaml:"vars,omitempty"`
	// Includes merges another devx.yaml's tasks into this one graph,
	// namespaced as "<name>:<task>". Not a recursive sub-process per file.
	Includes map[string]string `yaml:"includes,omitempty"`
	// Agents are named shell commands launchable via `devx workspace agent`.
	Agents map[string]string `yaml:"agents,omitempty"`
	// Tasks holds every other top-level key, keyed by its own task name
	// (not yet namespaced — see Load for the merged, namespaced result).
	Tasks map[string]Task `yaml:"-"`

	// dir is the directory this file lives in — the default working
	// directory for every task it directly defines. Not serialized.
	dir string
}

// Dir returns the directory the defining devx.yaml file lives in.
func (m *Manifest) Dir() string { return m.dir }

type Project struct {
	Name string `yaml:"name"`
}

// AIConfig holds optional AI provider settings used by 'devx ai',
// automatic connection string detection, and local inference setup.
// Credentials are read from environment variables:
//
//	openai:       OPENAI_API_KEY
//	anthropic:    ANTHROPIC_API_KEY
//	azure-openai: AZURE_OPENAI_KEY
//	ollama/local: no auth required
type AIConfig struct {
	Provider string         `yaml:"provider"` // openai | anthropic | ollama | azure-openai | local
	Model    string         `yaml:"model"`
	BaseURL  string         `yaml:"baseURL,omitempty"` // override endpoint
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

// ReadyCheck overrides how devx decides a detached task is ready. Exactly
// one of HTTP, TCP, or Cmd should be set; when all are empty, readiness is
// inferred from the task's run: text (see the Inference package doc).
type ReadyCheck struct {
	HTTP string `yaml:"http,omitempty"`
	TCP  string `yaml:"tcp,omitempty"`
	Cmd  string `yaml:"cmd,omitempty"`
}

// Task is either a bare string (just the command) or an object with any of
// the optional fields below. See docs/design/task-executor-refactor.md for
// the full field-by-field rationale.
type Task struct {
	Deps          []string          `yaml:"deps,omitempty"`
	Run           string            `yaml:"run,omitempty"`
	Ready         *ReadyCheck       `yaml:"ready,omitempty"`
	Detached      *bool             `yaml:"detached,omitempty"`
	Desc          string            `yaml:"desc,omitempty"`
	Group         string            `yaml:"group,omitempty"`
	Env           map[string]string `yaml:"env,omitempty"`
	OS            []string          `yaml:"os,omitempty"`
	Interactive   bool              `yaml:"interactive,omitempty"`
	Sources       []string          `yaml:"sources,omitempty"`
	Generates     []string          `yaml:"generates,omitempty"`
	Port          int               `yaml:"port,omitempty"`
	Timeout       string            `yaml:"timeout,omitempty"`
	Restart       string            `yaml:"restart,omitempty"` // never | on-failure | always
	StopTimeout   string            `yaml:"stopTimeout,omitempty"`
	Shell         string            `yaml:"shell,omitempty"`
	Dir           string            `yaml:"dir,omitempty"`
	Requires      []string          `yaml:"requires,omitempty"`
	ContainerName string            `yaml:"containerName,omitempty"`
	Scope         string            `yaml:"scope,omitempty"` // workspace (default) | shared

	// Name is the task's fully namespaced name (e.g. "infra:db"), filled
	// in by Load. Not set when decoding a raw file in isolation.
	Name string `yaml:"-"`
	// SourceDir is the directory of the devx.yaml file that defined this
	// task — the default working directory for Run/Sources/Generates/Dir.
	SourceDir string `yaml:"-"`
	// ScopeVars are the vars: visible to this task's own substitution pass:
	// its defining file's own vars: merged over the root manifest's vars:
	// (root as fallback, own file as override) — see "Composition".
	ScopeVars map[string]string `yaml:"-"`
}

// UnmarshalYAML accepts either a bare scalar (shorthand for {run: <scalar>})
// or a full mapping.
func (t *Task) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		t.Run = node.Value
		return nil
	}
	type rawTask Task
	var raw rawTask
	if err := node.Decode(&raw); err != nil {
		return err
	}
	*t = Task(raw)
	return nil
}

// UnmarshalYAML splits top-level keys between the fixed reserved fields and
// the dynamic set of task names, since task names are top-level keys
// directly rather than nested under a tasks:/services:/profiles: wrapper.
func (m *Manifest) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("devx.yaml: top level must be a mapping")
	}
	m.Tasks = map[string]Task{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode, valNode := node.Content[i], node.Content[i+1]
		key := keyNode.Value
		var err error
		switch key {
		case "version":
			err = valNode.Decode(&m.Version)
		case "project":
			err = valNode.Decode(&m.Project)
		case "ai":
			err = valNode.Decode(&m.AI)
		case "tools":
			err = valNode.Decode(&m.Tools)
		case "setup":
			err = valNode.Decode(&m.Setup)
		case "vars":
			err = valNode.Decode(&m.Vars)
		case "includes":
			err = valNode.Decode(&m.Includes)
		case "agents":
			err = valNode.Decode(&m.Agents)
		default:
			var t Task
			if decErr := valNode.Decode(&t); decErr != nil {
				err = fmt.Errorf("task %q: %w", key, decErr)
				break
			}
			t.Name = key
			m.Tasks[key] = t
		}
		if err != nil {
			return fmt.Errorf("devx.yaml: key %q: %w", key, err)
		}
	}
	return nil
}

// parseFile reads and parses a single devx.yaml file in isolation, without
// resolving includes:. dir is recorded as every task's default SourceDir.
func parseFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	m.dir = filepath.Dir(abs)
	return &m, nil
}

// Parse parses a single devx.yaml document from memory, without resolving
// includes: (any includes: entries are kept but not followed). Primarily
// for tests; use Load to resolve a real file's includes: graph.
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}
	return &m, nil
}

// Load reads path as the root manifest and merges every includes: file's
// tasks into it, namespaced as "<name>:<task>". Each task's ScopeVars is
// resolved to that file's own vars: merged over the root's vars: (root as
// fallback). Returns the merged Manifest with Tasks fully namespaced and
// SourceDir/ScopeVars set on every task.
func Load(path string) (*Manifest, error) {
	root, err := parseFile(path)
	if err != nil {
		return nil, err
	}

	merged := map[string]Task{}
	for name, t := range root.Tasks {
		t.Name = name
		t.SourceDir = root.dir
		t.ScopeVars = root.Vars
		merged[name] = t
	}

	for ns, incPath := range root.Includes {
		resolved := incPath
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(root.dir, incPath)
		}
		inc, err := parseFile(resolved)
		if err != nil {
			return nil, fmt.Errorf("includes.%s: %w", ns, err)
		}
		scopeVars := mergeVars(root.Vars, inc.Vars)
		for name, t := range inc.Tasks {
			namespaced := ns + ":" + name
			t.Name = namespaced
			t.SourceDir = inc.dir
			t.ScopeVars = scopeVars
			if _, collide := merged[namespaced]; collide {
				return nil, fmt.Errorf("task %q is defined more than once", namespaced)
			}
			merged[namespaced] = t
		}
	}

	root.Tasks = merged
	return root, nil
}

// mergeVars returns a new map with base entries overridden by override
// entries of the same key — base is the fallback layer.
func mergeVars(base, override map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}
