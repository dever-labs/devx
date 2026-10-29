package main

import (
	"strings"
	"testing"

	"github.com/dever-labs/devx/internal/localai"
)

func TestRunAIModelPull_MLXPrintsConvertInstructions(t *testing.T) {
	out := captureStdout(t, func() {
		if err := runAIModelPull(localai.BackendMLX, "mistral-7b"); err != nil {
			t.Fatalf("runAIModelPull(mlx): %v", err)
		}
	})
	if !strings.Contains(out, "mlx_lm.convert") {
		t.Errorf("expected MLX convert instructions, got:\n%s", out)
	}
}

func TestRunAIModelPull_RemotePrintsHint(t *testing.T) {
	out := captureStdout(t, func() {
		if err := runAIModelPull(localai.BackendRemote, "llama3"); err != nil {
			t.Fatalf("runAIModelPull(remote): %v", err)
		}
	})
	if !strings.Contains(out, "llama3") {
		t.Errorf("expected model name in remote hint, got:\n%s", out)
	}
}

func TestRunAIModelPull_OllamaNotInstalledErrors(t *testing.T) {
	captureStdout(t, func() {
		if err := runAIModelPull(localai.BackendOllama, "llama3"); err == nil {
			t.Error("expected error: ollama CLI is not installed in this sandbox")
		}
	})
}

func TestRunAIModelRemove_MLXPrintsRmInstructions(t *testing.T) {
	out := captureStdout(t, func() {
		if err := runAIModelRemove(localai.BackendMLX, "mistral-7b"); err != nil {
			t.Fatalf("runAIModelRemove(mlx): %v", err)
		}
	})
	if !strings.Contains(out, "rm -rf") || !strings.Contains(out, "mistral-7b") {
		t.Errorf("expected rm -rf instructions mentioning model name, got:\n%s", out)
	}
}

func TestRunAIModelRemove_RemotePrintsHint(t *testing.T) {
	out := captureStdout(t, func() {
		if err := runAIModelRemove(localai.BackendRemote, "llama3"); err != nil {
			t.Fatalf("runAIModelRemove(remote): %v", err)
		}
	})
	if !strings.Contains(out, "manage models") {
		t.Errorf("expected remote management hint, got:\n%s", out)
	}
}

func TestRunAIModelRemove_OllamaNotInstalledErrors(t *testing.T) {
	captureStdout(t, func() {
		if err := runAIModelRemove(localai.BackendOllama, "llama3"); err == nil {
			t.Error("expected error: ollama CLI is not installed in this sandbox")
		}
	})
}
