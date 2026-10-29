package localai

import (
	"os"
	"strings"
	"testing"
)

// chdirTemp chdirs into a fresh temp dir for the duration of the test,
// restoring the original working directory on cleanup. Mirrors the helper
// used in cmd/devx and internal/doctor tests (go.mod declares go 1.21, so
// t.Chdir from Go 1.24 isn't used).
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Logf("warning: could not restore working directory: %v", err)
		}
	})
	return dir
}

func TestLoadSavedAIConfig_AbsentReturnsNilNoError(t *testing.T) {
	chdirTemp(t)
	cfg, err := LoadSavedAIConfig()
	if err != nil {
		t.Fatalf("LoadSavedAIConfig: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil config when %s is absent, got %+v", AIConfigPath, cfg)
	}
}

func TestSaveAndLoadAIConfig_RoundTrips(t *testing.T) {
	chdirTemp(t)
	want := &SavedAIConfig{
		Provider:          "local",
		Tool:              "aider",
		Model:             "qwen2.5-coder:14b",
		Backend:           BackendOllama,
		AutocompleteModel: "qwen2.5-coder:1.5b",
	}
	if err := SaveAIConfig(want); err != nil {
		t.Fatalf("SaveAIConfig: %v", err)
	}

	got, err := LoadSavedAIConfig()
	if err != nil {
		t.Fatalf("LoadSavedAIConfig: %v", err)
	}
	if got == nil {
		t.Fatal("expected a non-nil config after save")
	}
	if *got != *want {
		t.Errorf("LoadSavedAIConfig() = %+v, want %+v", got, want)
	}
}

func TestLoadSavedAIConfig_MalformedYAMLErrors(t *testing.T) {
	chdirTemp(t)
	if err := os.MkdirAll(".devx", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(AIConfigPath, []byte("not: valid: yaml: ["), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSavedAIConfig(); err == nil {
		t.Fatal("expected an error for malformed YAML")
	}
}

func TestDeleteAIConfig_RemovesFileAndIsNoOpWhenAbsent(t *testing.T) {
	chdirTemp(t)
	if err := DeleteAIConfig(); err != nil {
		t.Fatalf("DeleteAIConfig on absent file: %v", err)
	}

	if err := SaveAIConfig(&SavedAIConfig{Provider: "local", Tool: "aider"}); err != nil {
		t.Fatalf("SaveAIConfig: %v", err)
	}
	if err := DeleteAIConfig(); err != nil {
		t.Fatalf("DeleteAIConfig: %v", err)
	}
	if _, err := os.Stat(AIConfigPath); !os.IsNotExist(err) {
		t.Errorf("expected %s to be removed, stat err = %v", AIConfigPath, err)
	}
}

func TestToolsForProvider_KnownAndDefault(t *testing.T) {
	cases := []struct {
		provider string
		wantID   string
	}{
		{"local", "aider"},
		{"anthropic", "claude-code"},
		{"openai", "codex"},
		{"copilot", "gh-copilot"},
		{"unknown-provider", "aider"},
	}
	for _, tc := range cases {
		tools := ToolsForProvider(tc.provider)
		if len(tools) == 0 {
			t.Errorf("ToolsForProvider(%q) returned no tools", tc.provider)
			continue
		}
		if tools[0].ID != tc.wantID {
			t.Errorf("ToolsForProvider(%q)[0].ID = %q, want %q", tc.provider, tools[0].ID, tc.wantID)
		}
	}
}

func TestSetenv_AddsNewAndReplacesExisting(t *testing.T) {
	env := []string{"FOO=bar", "BAZ=qux"}

	got := setenv(env, "FOO", "newval")
	if len(got) != 2 {
		t.Fatalf("expected setenv to replace in place, got %v", got)
	}
	if got[0] != "FOO=newval" {
		t.Errorf("got[0] = %q, want FOO=newval", got[0])
	}

	got2 := setenv(env, "NEW", "value")
	if len(got2) != 3 || got2[2] != "NEW=value" {
		t.Errorf("expected setenv to append a new var, got %v", got2)
	}
}

func TestAiderArgs_ProviderBranches(t *testing.T) {
	cases := []struct {
		name string
		cfg  *SavedAIConfig
		want []string
	}{
		{"no model", &SavedAIConfig{Provider: "local", Backend: BackendOllama}, nil},
		{"local ollama", &SavedAIConfig{Provider: "local", Backend: BackendOllama, Model: "qwen2.5-coder:14b"}, []string{"--model", "ollama/qwen2.5-coder:14b"}},
		{"local mlx", &SavedAIConfig{Provider: "local", Backend: BackendMLX, Model: "mistral"}, []string{"--model", "openai/mistral"}},
		{"anthropic", &SavedAIConfig{Provider: "anthropic", Model: "claude-sonnet-4-5"}, []string{"--model", "anthropic/claude-sonnet-4-5"}},
		{"openai", &SavedAIConfig{Provider: "openai", Model: "gpt-4o"}, []string{"--model", "gpt-4o"}},
		{"unknown provider", &SavedAIConfig{Provider: "copilot", Model: "whatever"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := aiderArgs(tc.cfg)
			if !equalStrSlices(got, tc.want) {
				t.Errorf("aiderArgs(%+v) = %v, want %v", tc.cfg, got, tc.want)
			}
		})
	}
}

func equalStrSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBuildToolCmd_UnknownToolErrors(t *testing.T) {
	_, err := buildToolCmd(&SavedAIConfig{Tool: "not-a-real-tool"})
	if err == nil {
		t.Fatal("expected an error for an unknown tool")
	}
}

func TestBuildToolCmd_ClaudeCodeAndGhCopilot(t *testing.T) {
	cmd, err := buildToolCmd(&SavedAIConfig{Tool: "claude-code"})
	if err != nil {
		t.Fatalf("buildToolCmd(claude-code): %v", err)
	}
	if !strings.HasSuffix(cmd.Path, "claude") && cmd.Args[0] != "claude" {
		t.Errorf("expected claude command, got %+v", cmd.Args)
	}

	cmd2, err := buildToolCmd(&SavedAIConfig{Tool: "gh-copilot"})
	if err != nil {
		t.Fatalf("buildToolCmd(gh-copilot): %v", err)
	}
	if len(cmd2.Args) < 3 || cmd2.Args[1] != "copilot" || cmd2.Args[2] != "suggest" {
		t.Errorf("expected 'gh copilot suggest' args, got %+v", cmd2.Args)
	}
}

func TestBuildToolCmd_AiderSetsOllamaEnv(t *testing.T) {
	cmd, err := buildToolCmd(&SavedAIConfig{
		Tool:     "aider",
		Provider: "local",
		Backend:  BackendOllama,
		Model:    "qwen2.5-coder:14b",
	})
	if err != nil {
		t.Fatalf("buildToolCmd(aider/ollama): %v", err)
	}
	if !hasEnvVar(cmd.Env, "OLLAMA_API_BASE=http://localhost:11434") {
		t.Errorf("expected OLLAMA_API_BASE to be set, env = %v", cmd.Env)
	}
}

func TestBuildToolCmd_AiderSetsMLXEnv(t *testing.T) {
	cmd, err := buildToolCmd(&SavedAIConfig{
		Tool:     "aider",
		Provider: "local",
		Backend:  BackendMLX,
		Model:    "mistral",
	})
	if err != nil {
		t.Fatalf("buildToolCmd(aider/mlx): %v", err)
	}
	if !hasEnvVar(cmd.Env, "OPENAI_API_BASE=http://localhost:8080/v1") {
		t.Errorf("expected OPENAI_API_BASE to be set for MLX, env = %v", cmd.Env)
	}
	if !hasEnvVar(cmd.Env, "OPENAI_API_KEY=local") {
		t.Errorf("expected OPENAI_API_KEY=local for MLX, env = %v", cmd.Env)
	}
}

func TestBuildToolCmd_AiderRemoteUsesEndpoint(t *testing.T) {
	cmd, err := buildToolCmd(&SavedAIConfig{
		Tool:     "aider",
		Provider: "remote",
		Endpoint: "http://ai.internal:11434",
		Model:    "llama3",
	})
	if err != nil {
		t.Fatalf("buildToolCmd(aider/remote): %v", err)
	}
	if !hasEnvVar(cmd.Env, "OPENAI_API_BASE=http://ai.internal:11434") {
		t.Errorf("expected OPENAI_API_BASE to use remote endpoint, env = %v", cmd.Env)
	}
}

func TestBuildToolCmd_CodexSetsLocalEnv(t *testing.T) {
	cmd, err := buildToolCmd(&SavedAIConfig{Tool: "codex", Provider: "local"})
	if err != nil {
		t.Fatalf("buildToolCmd(codex): %v", err)
	}
	if !hasEnvVar(cmd.Env, "OPENAI_API_BASE=http://localhost:8080/v1") {
		t.Errorf("expected codex local provider to set OPENAI_API_BASE, env = %v", cmd.Env)
	}
}

func hasEnvVar(env []string, kv string) bool {
	for _, e := range env {
		if e == kv {
			return true
		}
	}
	return false
}

func TestLaunchTool_LocalBackendNotRunningErrors(t *testing.T) {
	// No real backend is listening on the hardcoded localhost ports used by
	// probeMLX/probeOllama when host.docker.internal also fails — but since
	// this sandbox may have a live Ollama server reachable via
	// host.docker.internal, only assert the deterministic parts: either a
	// "not running" error, or (if a backend really is detected) that
	// LaunchTool proceeds to try executing the tool binary, which then
	// fails since the tool isn't installed. Either way it must return an
	// error and must not hang.
	cfg := &SavedAIConfig{Provider: "local", Backend: BackendOllama, Tool: "aider"}
	err := LaunchTool(cfg)
	if err == nil {
		t.Fatal("expected LaunchTool to fail: no real tool/backend available in this sandbox")
	}
}

func TestLaunchTool_RemoteUnreachableErrors(t *testing.T) {
	cfg := &SavedAIConfig{Provider: "remote", Backend: BackendRemote, Endpoint: "http://127.0.0.1:1", Tool: "aider"}
	err := LaunchTool(cfg)
	if err == nil {
		t.Fatal("expected an error for an unreachable remote endpoint")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Errorf("error = %q, want it to mention the backend is not running", err)
	}
}

func TestLaunchTool_UnknownToolErrors(t *testing.T) {
	// Provider "anthropic" skips the backend-detection branch entirely, so
	// this exercises buildToolCmd's unknown-tool error path directly.
	cfg := &SavedAIConfig{Provider: "anthropic", Tool: "not-a-real-tool"}
	if err := LaunchTool(cfg); err == nil {
		t.Fatal("expected an error for an unknown tool")
	}
}
