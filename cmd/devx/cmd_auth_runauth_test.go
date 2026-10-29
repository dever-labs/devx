package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunAuth_ListsAllChecks(t *testing.T) {
	var err error
	out := captureStdout(t, func() {
		err = runAuth(context.Background(), nil)
	})
	if err != nil {
		t.Fatalf("runAuth failed: %v", err)
	}
	for _, want := range []string{"GitHub CLI", "Anthropic", "OpenAI", "Azure OpenAI"} {
		if !strings.Contains(out, want) {
			t.Errorf("runAuth output missing %q, got:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "To add a provider") {
		t.Errorf("expected the closing hint line, got:\n%s", out)
	}
}
