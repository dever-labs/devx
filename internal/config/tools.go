package config

// Tool declares a required SDK, runtime, or CLI tool for the project.
// devx doctor checks each tool using its Check command and reports missing tools.
// devx init (or devx doctor --fix) installs missing tools using the Install block.
type Tool struct {
	Name    string  `yaml:"name"`
	Version string  `yaml:"version,omitempty"` // informational, shown in doctor output
	Check   string  `yaml:"check"`             // shell command to verify installation
	Install Install `yaml:"install,omitempty"`
}

// Install holds platform-specific install commands.
type Install struct {
	Windows string `yaml:"windows,omitempty"`
	MacOS   string `yaml:"macos,omitempty"`
	Linux   string `yaml:"linux,omitempty"`
}

// SetupStep is a host-side command run as part of `devx init`.
// Steps run in declaration order. RunOnce steps are skipped if their
// command hash matches a previous successful run stored in .devx/setup-state.json.
type SetupStep struct {
	Name     string `yaml:"name"`
	Run      string `yaml:"run"`
	Workdir  string `yaml:"workdir,omitempty"`  // working directory; defaults to cwd
	RunOnce  bool   `yaml:"runOnce,omitempty"`  // skip if hash matches last run
	Platform string `yaml:"platform,omitempty"` // all | windows | linux | macos (default: all)
}
