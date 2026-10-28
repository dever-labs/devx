package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/dever-labs/devx/internal/config"
)

// chdirTemp changes the working directory to a fresh temp dir and restores
// the original directory on test cleanup. Manually implemented (rather than
// t.Chdir) because go.mod declares go 1.21 and golangci-lint's stdversion
// check gates stdlib APIs against the declared directive.
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(old)
	})
	return dir
}

func TestHasFailures_Empty(t *testing.T) {
	r := Report{}
	if r.HasFailures() {
		t.Fatal("expected no failures in empty report")
	}
}

func TestHasFailures_AllPass(t *testing.T) {
	r := Report{Checks: []Check{
		{Name: "a", Status: "PASS"},
		{Name: "b", Status: "PASS"},
		{Name: "c", Status: "WARN"},
	}}
	if r.HasFailures() {
		t.Fatal("expected no failures when all checks are PASS or WARN")
	}
}

func TestHasFailures_WithFail(t *testing.T) {
	r := Report{Checks: []Check{
		{Name: "a", Status: "PASS"},
		{Name: "b", Status: "FAIL"},
	}}
	if !r.HasFailures() {
		t.Fatal("expected HasFailures to return true when a FAIL check is present")
	}
}

// ── PrintReport ──────────────────────────────────────────────────────────

func TestPrintReport_Human(t *testing.T) {
	report := Report{Checks: []Check{
		{Name: "CLI", Status: "PASS", Detail: "devx dev"},
		{Name: "Runtime: docker", Status: "FAIL", Detail: "docker not found"},
		{Name: "Ports", Status: "WARN", Detail: "port 8080 conflict"},
	}}

	// Capture stdout via a pipe.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w

	PrintReport(os.Stdout, report, false)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "✓") {
		t.Error("expected PASS icon ✓ in human output")
	}
	if !strings.Contains(output, "✗") {
		t.Error("expected FAIL icon ✗ in human output")
	}
	if !strings.Contains(output, "!") {
		t.Error("expected WARN icon ! in human output")
	}
	if !strings.Contains(output, "docker not found") {
		t.Error("expected detail text in human output")
	}
}

func TestPrintReport_JSON(t *testing.T) {
	report := Report{Checks: []Check{
		{Name: "CLI", Status: "PASS", Detail: "devx dev"},
		{Name: "Runtime: docker", Status: "FAIL"},
	}}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w

	PrintReport(os.Stdout, report, true)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	var checks []Check
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &checks); err != nil {
		t.Fatalf("JSON output is not valid JSON: %v\noutput: %s", err, output)
	}
	if len(checks) != 2 {
		t.Fatalf("expected 2 checks in JSON, got %d", len(checks))
	}
	if checks[0].Name != "CLI" {
		t.Errorf("expected first check name 'CLI', got %q", checks[0].Name)
	}
	if checks[1].Status != "FAIL" {
		t.Errorf("expected second check status 'FAIL', got %q", checks[1].Status)
	}
}

func TestPrintReport_JSON_OmitsEmptyDetail(t *testing.T) {
	report := Report{Checks: []Check{
		{Name: "X", Status: "PASS", Detail: ""},
	}}

	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	PrintReport(os.Stdout, report, true)
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	// Detail field has omitempty — must not appear in output when empty.
	if strings.Contains(buf.String(), `"detail"`) {
		t.Error("expected empty detail to be omitted from JSON")
	}
}

// ── doctorIcon ──────────────────────────────────────────────────────────

func TestDoctorIcon(t *testing.T) {
	cases := []struct {
		status string
		want   string
	}{
		{"PASS", "✓"},
		{"WARN", "!"},
		{"FAIL", "✗"},
		{"", "✗"},
		{"OTHER", "✗"},
	}
	for _, tc := range cases {
		got := doctorIcon(tc.status)
		if got != tc.want {
			t.Errorf("doctorIcon(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

// ── checkPortConflicts ───────────────────────────────────────────────────

func TestCheckPortConflicts_NoDuplicates(t *testing.T) {
	m := &config.Manifest{Tasks: map[string]config.Task{
		"web": {Name: "web", Run: "echo web", Port: 8080},
		"api": {Name: "api", Run: "echo api", Port: 8081},
	}}
	checks := checkPortConflicts(m)
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d: %+v", len(checks), checks)
	}
	if checks[0].Status != "PASS" {
		t.Errorf("expected PASS for non-conflicting ports, got %+v", checks[0])
	}
}

func TestCheckPortConflicts_DuplicatePort(t *testing.T) {
	m := &config.Manifest{Tasks: map[string]config.Task{
		"web":  {Name: "web", Run: "echo web", Port: 8080},
		"web2": {Name: "web2", Run: "echo web2", Port: 8080},
	}}
	checks := checkPortConflicts(m)
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d: %+v", len(checks), checks)
	}
	if checks[0].Status != "WARN" {
		t.Errorf("expected WARN for duplicate ports, got %+v", checks[0])
	}
	if !strings.Contains(checks[0].Detail, "8080") || !strings.Contains(checks[0].Detail, "web") {
		t.Errorf("expected detail to mention the port and task names, got %q", checks[0].Detail)
	}
}

// ── checkTaskRequires ────────────────────────────────────────────────────

func TestCheckTaskRequires_AllSatisfied(t *testing.T) {
	m := &config.Manifest{Tasks: map[string]config.Task{
		"web": {Name: "web", Run: "echo web", Requires: []string{"sh"}},
	}}
	checks := checkTaskRequires(m)
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d: %+v", len(checks), checks)
	}
	if checks[0].Status != "PASS" {
		t.Errorf("expected PASS for a binary on PATH, got %+v", checks[0])
	}
}

func TestCheckTaskRequires_Missing(t *testing.T) {
	m := &config.Manifest{Tasks: map[string]config.Task{
		"web": {Name: "web", Run: "echo web", Requires: []string{"definitely-not-a-real-binary-xyz"}},
	}}
	checks := checkTaskRequires(m)
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d: %+v", len(checks), checks)
	}
	if checks[0].Status != "FAIL" {
		t.Errorf("expected FAIL for a missing binary, got %+v", checks[0])
	}
	if !strings.Contains(checks[0].Detail, "web") || !strings.Contains(checks[0].Detail, "definitely-not-a-real-binary-xyz") {
		t.Errorf("expected detail to mention the task and binary name, got %q", checks[0].Detail)
	}
}

func TestCheckTaskRequires_NoRequires(t *testing.T) {
	m := &config.Manifest{Tasks: map[string]config.Task{
		"web": {Name: "web", Run: "echo web"},
	}}
	checks := checkTaskRequires(m)
	if len(checks) != 0 {
		t.Errorf("expected no checks when no task declares requires:, got %+v", checks)
	}
}

// ── checkTools ───────────────────────────────────────────────────────────

func TestCheckTools_NoToolsReturnsNil(t *testing.T) {
	m := &config.Manifest{}
	checks := checkTools(context.Background(), m, false)
	if checks != nil {
		t.Errorf("expected nil checks when manifest declares no tools, got %+v", checks)
	}
}

// ── checkDevcontainerCLI ─────────────────────────────────────────────────

func TestCheckDevcontainerCLI_ReturnsPassOrWarn(t *testing.T) {
	check := checkDevcontainerCLI()
	if check.Name != "devcontainer CLI" {
		t.Errorf("expected check name 'devcontainer CLI', got %q", check.Name)
	}
	if check.Status != "PASS" && check.Status != "WARN" {
		t.Errorf("expected PASS or WARN, got %q", check.Status)
	}
}

// ── checkAI ──────────────────────────────────────────────────────────────

func TestCheckAI_NotConfigured(t *testing.T) {
	chdirTemp(t)
	checks := checkAI()
	if len(checks) != 1 {
		t.Fatalf("expected 1 check when no .devx/ai.yaml exists, got %+v", checks)
	}
	if checks[0].Status != "WARN" {
		t.Errorf("expected WARN when AI is not configured, got %+v", checks[0])
	}
	if !strings.Contains(checks[0].Detail, "not configured") {
		t.Errorf("expected detail to mention 'not configured', got %q", checks[0].Detail)
	}
}

func TestCheckAI_MalformedConfig(t *testing.T) {
	chdirTemp(t)
	if err := os.MkdirAll(".devx", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".devx/ai.yaml", []byte("not: [valid: yaml"), 0644); err != nil {
		t.Fatal(err)
	}
	checks := checkAI()
	if len(checks) != 1 {
		t.Fatalf("expected 1 check for malformed config, got %+v", checks)
	}
	if checks[0].Status != "WARN" {
		t.Errorf("expected WARN for malformed ai.yaml, got %+v", checks[0])
	}
}

func TestCheckAI_ConfiguredNoProbe(t *testing.T) {
	chdirTemp(t)
	if err := os.MkdirAll(".devx", 0755); err != nil {
		t.Fatal(err)
	}
	// Provider other than "local"/"remote" skips the backend probe, and an
	// unrecognized tool name skips the binary-on-PATH check — keeping this
	// test hermetic (no real AI backend/tool required).
	yaml := "provider: cloud\ntool: unknown-tool\nmodel: gpt-4\n"
	if err := os.WriteFile(".devx/ai.yaml", []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	checks := checkAI()
	if len(checks) != 1 {
		t.Fatalf("expected 1 check (config only, no probe), got %+v", checks)
	}
	if checks[0].Status != "PASS" {
		t.Errorf("expected PASS for a well-formed ai.yaml, got %+v", checks[0])
	}
	if !strings.Contains(checks[0].Detail, "cloud") || !strings.Contains(checks[0].Detail, "gpt-4") {
		t.Errorf("expected detail to mention provider/model, got %q", checks[0].Detail)
	}
}
