package main

import (
	"encoding/json"
	"testing"
)

func TestShortFeatureName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"ghcr.io/devcontainers/features/go:1", "go:1"},
		{"node:18", "node:18"},
	}
	for _, tc := range cases {
		if got := shortFeatureName(tc.in); got != tc.want {
			t.Errorf("shortFeatureName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestReCapture(t *testing.T) {
	s := "source=myvol,target=/data,type=volume"
	if got := reCapture(s, `source=([^,]+)`); got != "myvol" {
		t.Errorf("reCapture(source) = %q, want %q", got, "myvol")
	}
	if got := reCapture(s, `target=([^,]+)`); got != "/data" {
		t.Errorf("reCapture(target) = %q, want %q", got, "/data")
	}
	if got := reCapture(s, `missing=([^,]+)`); got != "" {
		t.Errorf("reCapture(missing) = %q, want empty", got)
	}
}

func TestMountSummary_StringVolume(t *testing.T) {
	m := "source=myvol,target=/data,type=volume"
	got := mountSummary(m)
	want := "/data  (volume: myvol)"
	if got != want {
		t.Errorf("mountSummary() = %q, want %q", got, want)
	}
}

func TestMountSummary_StringBind(t *testing.T) {
	m := "source=/host/path,target=/data,type=bind"
	got := mountSummary(m)
	want := "/data"
	if got != want {
		t.Errorf("mountSummary() = %q, want %q", got, want)
	}
}

func TestMountSummary_StringNoTarget(t *testing.T) {
	m := "some-opaque-mount-string"
	if got := mountSummary(m); got != m {
		t.Errorf("mountSummary() = %q, want the original string %q", got, m)
	}
}

func TestMountSummary_NonString(t *testing.T) {
	m := map[string]interface{}{"source": "a", "target": "b"}
	got := mountSummary(m)
	if got == "" {
		t.Error("expected a non-empty JSON fallback for a non-string mount")
	}
}

func TestCommandString(t *testing.T) {
	if got := commandString(nil); got != "" {
		t.Errorf("commandString(nil) = %q, want empty", got)
	}
	if got := commandString("echo hi"); got != "echo hi" {
		t.Errorf("commandString(string) = %q, want %q", got, "echo hi")
	}
	if got := commandString([]interface{}{"echo", "hi"}); got != "echo hi" {
		t.Errorf("commandString([]interface{}) = %q, want %q", got, "echo hi")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate(short string) = %q, want unchanged", got)
	}
	if got := truncate("a very long string indeed", 10); got != "a very lon…" {
		t.Errorf("truncate(long string) = %q, want %q", got, "a very lon…")
	}
}

func TestStripJSONComments(t *testing.T) {
	in := []byte("{\n  // a comment\n  \"name\": \"test\" // trailing\n}\n")
	out := stripJSONComments(in)
	var v map[string]string
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("stripJSONComments output is not valid JSON: %v\noutput: %s", err, out)
	}
	if v["name"] != "test" {
		t.Errorf("expected name=test, got %v", v)
	}
}

func TestLifecycleCommands(t *testing.T) {
	cfg := &devcontainerJSON{
		InitializeCommand: "echo init",
		PostCreateCommand: "echo create",
		PostStartCommand:  "echo start",
	}
	got := lifecycleCommands(cfg)
	if len(got) != 3 {
		t.Fatalf("expected 3 lifecycle commands, got %v", got)
	}
}

func TestLifecycleCommands_Empty(t *testing.T) {
	cfg := &devcontainerJSON{}
	got := lifecycleCommands(cfg)
	if len(got) != 0 {
		t.Errorf("expected no lifecycle commands for an empty config, got %v", got)
	}
}

func TestVSCodeExtensions(t *testing.T) {
	cfg := &devcontainerJSON{
		Customizations: map[string]json.RawMessage{
			"vscode": json.RawMessage(`{"extensions": ["golang.go", "esbenp.prettier-vscode"]}`),
		},
	}
	got := vscodeExtensions(cfg)
	want := []string{"golang.go", "esbenp.prettier-vscode"}
	if len(got) != len(want) {
		t.Fatalf("vscodeExtensions() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("vscodeExtensions()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestVSCodeExtensions_NoCustomizations(t *testing.T) {
	cfg := &devcontainerJSON{}
	if got := vscodeExtensions(cfg); got != nil {
		t.Errorf("vscodeExtensions() = %v, want nil", got)
	}
}
