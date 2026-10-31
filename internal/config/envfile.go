package config

// EnvFile declares a local config/secrets file to scaffold from a template
// on first `devx init`, so cloning a repo doesn't require manually
// copy-pasting values out of a secrets doc. If Output already exists it is
// left untouched — scaffolding only ever creates the file, never
// overwrites a developer's already-filled-in local values.
type EnvFile struct {
	// Name is a human-readable label shown in `devx init` output.
	// Defaults to Output when empty.
	Name string `yaml:"name,omitempty"`
	// Template is a path (relative to this devx.yaml) to a Go
	// text/template file. Vars are available as {{.VAR_NAME}}.
	Template string `yaml:"template"`
	// Output is the path (relative to this devx.yaml) the rendered
	// template is written to. Skipped entirely if this file already exists.
	Output string `yaml:"output"`
	// Mode controls what happens when Output already exists: "" (default)
	// leaves it untouched entirely; "merge" renders the template and
	// appends any keys present in it but missing from Output, without
	// ever overwriting a key the file already has. Both Output and any
	// merged-in file are expected to be simple KEY=VALUE lines (.env
	// style); merge is a no-op on a file it can't parse that way.
	Mode string `yaml:"mode,omitempty"`
	// Vars declares each template placeholder and how to resolve its value.
	Vars []EnvVar `yaml:"vars"`
}

// EnvVar declares one template placeholder and the order devx resolves its
// value in: an OS environment variable, then (if running interactively) a
// terminal prompt, then Default. It's an error to have no value from any
// of those sources.
type EnvVar struct {
	// Name is the template placeholder name, e.g. "DB_PASSWORD" for
	// {{.DB_PASSWORD}}.
	Name string `yaml:"name"`
	// EnvVar is the OS environment variable checked first. Defaults to Name.
	EnvVar string `yaml:"envVar,omitempty"`
	// Prompt is the interactive prompt label. Defaults to Name.
	Prompt string `yaml:"prompt,omitempty"`
	// Default is used when no environment variable is set and (not
	// running interactively, or the prompt is left blank).
	Default string `yaml:"default,omitempty"`
	// Secret marks this value as sensitive for documentation purposes —
	// devx does not log it, but terminal input is not hidden.
	Secret bool `yaml:"secret,omitempty"`
}
