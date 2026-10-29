package main

import (
	"strings"
	"testing"
)

func TestRunCompletion_NoArgsReturnsError(t *testing.T) {
	if err := runCompletion(nil); err == nil {
		t.Fatal("expected an error when no shell is given")
	}
}

func TestRunCompletion_UnknownShell(t *testing.T) {
	if err := runCompletion([]string{"powershell"}); err == nil {
		t.Fatal("expected an error for an unsupported shell")
	}
}

func TestRunCompletion_KnownShells(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			var runErr error
			out := captureStdout(t, func() {
				runErr = runCompletion([]string{shell})
			})
			if runErr != nil {
				t.Fatalf("runCompletion(%q) failed: %v", shell, runErr)
			}
			if out == "" {
				t.Errorf("runCompletion(%q) produced no output", shell)
			}
		})
	}
}

func TestBashCompletion_MentionsAllCommands(t *testing.T) {
	out := bashCompletion()
	for _, cmd := range allCommands {
		if !strings.Contains(out, cmd) {
			t.Errorf("bashCompletion() missing command %q", cmd)
		}
	}
}

func TestZshCompletion_MentionsAllCommands(t *testing.T) {
	out := zshCompletion()
	for _, cmd := range allCommands {
		if !strings.Contains(out, cmd) {
			t.Errorf("zshCompletion() missing command %q", cmd)
		}
	}
}

func TestFishCompletion_MentionsAllCommands(t *testing.T) {
	out := fishCompletion()
	for _, cmd := range allCommands {
		if !strings.Contains(out, cmd) {
			t.Errorf("fishCompletion() missing command %q", cmd)
		}
	}
}
