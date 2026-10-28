package main

import (
	"bufio"
	"strings"
	"testing"

	"github.com/dever-labs/devx/internal/localai"
)

func TestPromptChoice_DefaultOnEmptyInput(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\n"))
	var idx int
	var err error
	_ = captureStdout(t, func() {
		idx, err = promptChoice(reader, "Pick one", []string{"a", "b", "c"}, 1)
	})
	if err != nil {
		t.Fatalf("promptChoice failed: %v", err)
	}
	if idx != 1 {
		t.Errorf("promptChoice with empty input = %d, want default 1", idx)
	}
}

func TestPromptChoice_ValidSelection(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("3\n"))
	var idx int
	var err error
	_ = captureStdout(t, func() {
		idx, err = promptChoice(reader, "Pick one", []string{"a", "b", "c"}, 0)
	})
	if err != nil {
		t.Fatalf("promptChoice failed: %v", err)
	}
	if idx != 2 {
		t.Errorf("promptChoice(\"3\") = %d, want 2 (0-indexed)", idx)
	}
}

func TestPromptChoice_InvalidFallsBackToDefault(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("99\n"))
	var idx int
	var err error
	_ = captureStdout(t, func() {
		idx, err = promptChoice(reader, "Pick one", []string{"a", "b", "c"}, 0)
	})
	if err != nil {
		t.Fatalf("promptChoice failed: %v", err)
	}
	if idx != 0 {
		t.Errorf("promptChoice(\"99\") = %d, want fallback to default 0", idx)
	}
}

func TestPromptString_DefaultOnEmptyInput(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\n"))
	var val string
	var err error
	_ = captureStdout(t, func() {
		val, err = promptString(reader, "Name", "my-app")
	})
	if err != nil {
		t.Fatalf("promptString failed: %v", err)
	}
	if val != "my-app" {
		t.Errorf("promptString with empty input = %q, want default %q", val, "my-app")
	}
}

func TestPromptString_UsesProvidedValue(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("custom-name\n"))
	var val string
	var err error
	_ = captureStdout(t, func() {
		val, err = promptString(reader, "Name", "my-app")
	})
	if err != nil {
		t.Fatalf("promptString failed: %v", err)
	}
	if val != "custom-name" {
		t.Errorf("promptString = %q, want %q", val, "custom-name")
	}
}

func TestPromptYN_DefaultOnEmptyInput(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\n"))
	var val bool
	var err error
	_ = captureStdout(t, func() {
		val, err = promptYN(reader, "Continue?", true)
	})
	if err != nil {
		t.Fatalf("promptYN failed: %v", err)
	}
	if !val {
		t.Error("promptYN with empty input should use the default (true)")
	}
}

func TestPromptYN_ParsesYesAndNo(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"y\n", true},
		{"yes\n", true},
		{"n\n", false},
		{"no\n", false},
		{"Y\n", true},
	}
	for _, tc := range cases {
		reader := bufio.NewReader(strings.NewReader(tc.input))
		var val bool
		var err error
		_ = captureStdout(t, func() {
			val, err = promptYN(reader, "Continue?", false)
		})
		if err != nil {
			t.Fatalf("promptYN(%q) failed: %v", tc.input, err)
		}
		if val != tc.want {
			t.Errorf("promptYN(%q) = %v, want %v", tc.input, val, tc.want)
		}
	}
}

func TestPrintYAMLSnippet_IncludesProviderAndModel(t *testing.T) {
	cfg := &localai.SavedAIConfig{Provider: "local", Model: "llama3", Backend: "ollama"}
	out := captureStdout(t, func() {
		printYAMLSnippet(cfg)
	})
	for _, want := range []string{"provider: local", "model: llama3", "backend: ollama"} {
		if !strings.Contains(out, want) {
			t.Errorf("printYAMLSnippet output missing %q, got:\n%s", want, out)
		}
	}
}

func TestPrintYAMLSnippet_OmitsLocalBlockForNonLocal(t *testing.T) {
	cfg := &localai.SavedAIConfig{Provider: "anthropic", Model: "claude"}
	out := captureStdout(t, func() {
		printYAMLSnippet(cfg)
	})
	if strings.Contains(out, "local:") {
		t.Errorf("expected no 'local:' block for provider=anthropic, got:\n%s", out)
	}
}

func TestPrintAIUsage_RunsWithoutPanicking(t *testing.T) {
	out := captureStdout(t, printAIUsage)
	if !strings.Contains(out, "devx ai") {
		t.Errorf("expected printAIUsage output to mention 'devx ai', got:\n%s", out)
	}
}
