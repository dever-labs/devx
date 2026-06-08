package main

import (
	"fmt"
	"os"
	"strings"
)

// allCommands is the canonical list used by completion scripts.
var allCommands = []string{
	"init", "setup", "up", "down", "status", "logs", "exec",
	"doctor", "validate", "render", "lock", "providers", "export",
	"run", "clone", "dev", "ai", "update", "completion", "version", "help",
}

var aiSubcommands = []string{"setup", "status", "reset"}
var devSubcommands = []string{"build", "rebuild", "up", "open", "exec"}
var renderSubcommands = []string{"compose", "k8s"}
var providersSubcommands = []string{"install", "list"}
var completionShells = []string{"bash", "zsh", "fish"}

func runCompletion(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: devx completion <bash|zsh|fish>")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Install completions:")
		fmt.Fprintln(os.Stderr, "  bash  — devx completion bash > /etc/bash_completion.d/devx")
		fmt.Fprintln(os.Stderr, `         or: devx completion bash >> ~/.bashrc`)
		fmt.Fprintln(os.Stderr, "  zsh   — devx completion zsh > ~/.zsh/completions/_devx")
		fmt.Fprintln(os.Stderr, `         or: devx completion zsh >> ~/.zshrc`)
		fmt.Fprintln(os.Stderr, "  fish  — devx completion fish > ~/.config/fish/completions/devx.fish")
		return fmt.Errorf("shell required")
	}

	switch args[0] {
	case "bash":
		fmt.Print(bashCompletion())
	case "zsh":
		fmt.Print(zshCompletion())
	case "fish":
		fmt.Print(fishCompletion())
	default:
		return fmt.Errorf("unknown shell %q — choose bash, zsh, or fish", args[0])
	}
	return nil
}

func bashCompletion() string {
	cmds := strings.Join(allCommands, " ")
	return fmt.Sprintf(`# devx bash completion
# Source this file or add to /etc/bash_completion.d/devx

_devx_completions() {
  local cur prev words
  COMPREPLY=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"

  case "$prev" in
    devx)
      COMPREPLY=( $(compgen -W "%s" -- "$cur") )
      return ;;
    ai)
      COMPREPLY=( $(compgen -W "%s" -- "$cur") )
      return ;;
    dev)
      COMPREPLY=( $(compgen -W "%s" -- "$cur") )
      return ;;
    render)
      COMPREPLY=( $(compgen -W "%s" -- "$cur") )
      return ;;
    providers)
      COMPREPLY=( $(compgen -W "%s" -- "$cur") )
      return ;;
    completion)
      COMPREPLY=( $(compgen -W "%s" -- "$cur") )
      return ;;
  esac
}

complete -F _devx_completions devx
`,
		cmds,
		strings.Join(aiSubcommands, " "),
		strings.Join(devSubcommands, " "),
		strings.Join(renderSubcommands, " "),
		strings.Join(providersSubcommands, " "),
		strings.Join(completionShells, " "),
	)
}

func zshCompletion() string {
	cmds := strings.Join(allCommands, "\n    ")
	return fmt.Sprintf(`#compdef devx
# devx zsh completion
# Place in a directory on your $fpath, e.g. ~/.zsh/completions/_devx

_devx() {
  local -a commands
  commands=(
    %s
  )

  local -a ai_cmds dev_cmds
  ai_cmds=(%s)
  dev_cmds=(%s)

  case $words[2] in
    ai)        _describe 'ai subcommand' ai_cmds ;;
    dev)       _describe 'dev subcommand' dev_cmds ;;
    *)         _describe 'devx command' commands ;;
  esac
}

_devx "$@"
`,
		cmds,
		strings.Join(aiSubcommands, " "),
		strings.Join(devSubcommands, " "),
	)
}

func fishCompletion() string {
	var lines []string
	lines = append(lines, "# devx fish completion")
	lines = append(lines, "# Place in ~/.config/fish/completions/devx.fish")
	lines = append(lines, "")

	for _, cmd := range allCommands {
		lines = append(lines, fmt.Sprintf(
			"complete -c devx -f -n '__fish_use_subcommand' -a %s", cmd,
		))
	}

	lines = append(lines, "")
	for _, sub := range aiSubcommands {
		lines = append(lines, fmt.Sprintf(
			"complete -c devx -f -n '__fish_seen_subcommand_from ai' -a %s", sub,
		))
	}
	for _, sub := range devSubcommands {
		lines = append(lines, fmt.Sprintf(
			"complete -c devx -f -n '__fish_seen_subcommand_from dev' -a %s", sub,
		))
	}

	return strings.Join(lines, "\n") + "\n"
}
