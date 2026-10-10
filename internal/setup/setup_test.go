package setup

import (
	"context"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/dever-labs/devx/internal/config"
)

// alwaysSucceedCmd returns a shell command that always exits 0.
func alwaysSucceedCmd() string {
	if goruntime.GOOS == "windows" {
		return "cmd /c exit 0"
	}
	return "true"
}

// alwaysFailCmd returns a shell command that always exits non-zero.
func alwaysFailCmd() string {
	if goruntime.GOOS == "windows" {
		return "cmd /c exit 1"
	}
	return "false"
}

// echoCmd returns a cross-platform echo command.
func echoCmd(msg string) string {
	if goruntime.GOOS == "windows" {
		return "echo " + msg
	}
	return "echo " + msg
}

func TestStepHash_Deterministic(t *testing.T) {
	h1 := stepHash("restore", "npm install", "./frontend")
	h2 := stepHash("restore", "npm install", "./frontend")
	if h1 != h2 {
		t.Fatalf("hash is not deterministic: %s != %s", h1, h2)
	}
}

func TestStepHash_DifferentOnChange(t *testing.T) {
	h1 := stepHash("restore", "npm install", "./frontend")
	h2 := stepHash("restore", "npm ci", "./frontend")
	if h1 == h2 {
		t.Fatal("expected different hashes for different commands")
	}
}

func TestStepHash_DifferentOnWorkdirChange(t *testing.T) {
	h1 := stepHash("restore", "npm install", "./frontend")
	h2 := stepHash("restore", "npm install", "./backend")
	if h1 == h2 {
		t.Fatal("expected different hashes for different workdirs")
	}
}

func TestStepHash_DifferentOnNameChange(t *testing.T) {
	h1 := stepHash("restore", "npm install", "")
	h2 := stepHash("build", "npm install", "")
	if h1 == h2 {
		t.Fatal("expected different hashes for different names")
	}
}

func TestLoadState_MissingFile(t *testing.T) {
	orig := stateFile
	stateFile = filepath.Join(t.TempDir(), "setup-state.json")
	defer func() { stateFile = orig }()

	s := loadState()
	if s == nil {
		t.Fatal("expected non-nil state")
	}
	if s.Steps == nil {
		t.Fatal("expected initialised Steps map")
	}
}

func TestLoadState_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	orig := stateFile
	stateFile = filepath.Join(dir, "setup-state.json")
	defer func() { stateFile = orig }()

	if err := os.WriteFile(stateFile, []byte("{{not valid json{{"), 0600); err != nil {
		t.Fatal(err)
	}

	s := loadState()
	if s == nil || s.Steps == nil {
		t.Fatal("expected fallback to empty state on corrupt JSON")
	}
}

func TestSaveAndLoadState(t *testing.T) {
	dir := t.TempDir()
	orig := stateFile
	stateFile = filepath.Join(dir, ".devx", "setup-state.json")
	defer func() { stateFile = orig }()

	s := &State{Steps: map[string]StepRecord{}}
	markDone(s, "restore", "npm install", "./frontend")

	if err := saveState(s); err != nil {
		t.Fatalf("saveState failed: %v", err)
	}
	if _, err := os.Stat(stateFile); err != nil {
		t.Fatalf("state file not written: %v", err)
	}

	s2 := loadState()
	if !hasRunBefore(s2, "restore", "npm install", "./frontend") {
		t.Fatal("expected step to be marked as run")
	}
	if hasRunBefore(s2, "restore", "npm ci", "./frontend") {
		t.Fatal("expected step with different command to NOT be marked as run")
	}
}

func TestSaveState_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	orig := stateFile
	// Nested dir that doesn't exist yet.
	stateFile = filepath.Join(dir, "a", "b", "c", "setup-state.json")
	defer func() { stateFile = orig }()

	s := &State{Steps: map[string]StepRecord{}}
	if err := saveState(s); err != nil {
		t.Fatalf("saveState should create parent directories: %v", err)
	}
	if _, err := os.Stat(stateFile); err != nil {
		t.Fatalf("state file not created: %v", err)
	}
}

func TestMarkDone_UpdatesExistingRecord(t *testing.T) {
	s := &State{Steps: map[string]StepRecord{}}
	markDone(s, "step", "npm install", "")
	first := s.Steps["step"].LastRun

	markDone(s, "step", "npm ci", "") // different command = different hash
	second := s.Steps["step"].LastRun

	if s.Steps["step"].Hash == stepHash("step", "npm install", "") {
		t.Fatal("expected hash to be updated to new command")
	}
	_ = first
	_ = second
}

func TestMatchesPlatform_AlwaysMatch(t *testing.T) {
	if !matchesPlatform("") {
		t.Fatal("empty platform must always match")
	}
	if !matchesPlatform("all") {
		t.Fatal("'all' must always match")
	}
}

func TestMatchesPlatform_CurrentOS(t *testing.T) {
	var currentPlatform string
	switch goruntime.GOOS {
	case "windows":
		currentPlatform = "windows"
	case "darwin":
		currentPlatform = "macos"
	default:
		currentPlatform = "linux"
	}
	if !matchesPlatform(currentPlatform) {
		t.Fatalf("expected current OS %q to match platform %q", goruntime.GOOS, currentPlatform)
	}
}

func TestMatchesPlatform_OtherOS(t *testing.T) {
	// Pick a platform that is definitely NOT the current OS.
	var otherPlatform string
	if goruntime.GOOS == "windows" {
		otherPlatform = "linux"
	} else {
		otherPlatform = "windows"
	}
	if matchesPlatform(otherPlatform) {
		t.Fatalf("expected platform %q to NOT match on %s", otherPlatform, goruntime.GOOS)
	}
}

func TestMatchesPlatform_Unknown(t *testing.T) {
	if matchesPlatform("freebsd") {
		t.Fatal("expected unknown platform to return false")
	}
	if matchesPlatform("solaris") {
		t.Fatal("expected unknown platform to return false")
	}
}

func TestMatchesPlatform_DarwinAlias(t *testing.T) {
	// "darwin" and "macos" must resolve identically.
	got1 := matchesPlatform("darwin")
	got2 := matchesPlatform("macos")
	if got1 != got2 {
		t.Fatalf("'darwin' and 'macos' must match the same way, got %v vs %v", got1, got2)
	}
}

func TestPlatformInstall_ReturnsCurrentOS(t *testing.T) {
	install := config.Install{
		Windows: "winget install x",
		MacOS:   "brew install x",
		Linux:   "apt-get install x",
	}
	got := platformInstall(install)
	switch goruntime.GOOS {
	case "windows":
		if got != install.Windows {
			t.Fatalf("expected Windows command, got %q", got)
		}
	case "darwin":
		if got != install.MacOS {
			t.Fatalf("expected macOS command, got %q", got)
		}
	default:
		if got != install.Linux {
			t.Fatalf("expected Linux command, got %q", got)
		}
	}
}

func TestPlatformInstall_Empty(t *testing.T) {
	got := platformInstall(config.Install{})
	if got != "" {
		t.Fatalf("expected empty string for empty install block, got %q", got)
	}
}

// ── CheckTool ──────────────────────────────────────────────────────────────

func TestCheckTool_Present(t *testing.T) {
	tool := config.Tool{Name: "echo", Check: echoCmd("hello")}
	ok, detail := CheckTool(tool)
	if !ok {
		t.Fatalf("expected tool to be present, got detail: %s", detail)
	}
}

func TestCheckTool_Missing(t *testing.T) {
	tool := config.Tool{Name: "nonexistent", Check: "__devx_definitely_not_a_real_tool_xyz__"}
	ok, _ := CheckTool(tool)
	if ok {
		t.Fatal("expected tool to be missing")
	}
}

func TestCheckTool_EmptyCheck(t *testing.T) {
	tool := config.Tool{Name: "unnamed"}
	ok, detail := CheckTool(tool)
	if ok {
		t.Fatal("expected false when check command is empty")
	}
	if detail != "no check command defined" {
		t.Fatalf("unexpected detail: %q", detail)
	}
}

func TestCheckTool_FailingCommand(t *testing.T) {
	tool := config.Tool{Name: "bad", Check: alwaysFailCmd()}
	ok, _ := CheckTool(tool)
	if ok {
		t.Fatal("expected failing command to report tool missing")
	}
}

// ── RunTools ──────────────────────────────────────────────────────────────

func TestRunTools_AllPresent(t *testing.T) {
	tools := []config.Tool{
		{Name: "t1", Check: alwaysSucceedCmd()},
		{Name: "t2", Check: alwaysSucceedCmd()},
	}
	results := RunTools(context.Background(), tools, Options{})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Status != "ok" {
			t.Errorf("tool %q: expected status 'ok', got %q", r.Name, r.Status)
		}
		if r.Type != "tool" {
			t.Errorf("tool %q: expected type 'tool', got %q", r.Name, r.Type)
		}
	}
}

func TestRunTools_Missing_NoFix(t *testing.T) {
	tools := []config.Tool{
		{Name: "missing-tool", Check: "__devx_definitely_not_a_real_tool_xyz__"},
	}
	results := RunTools(context.Background(), tools, Options{Fix: false})
	if len(results) != 1 || results[0].Status != "missing" {
		t.Fatalf("expected status 'missing', got %+v", results)
	}
}

func TestRunTools_Missing_Fix_NoInstallCmd(t *testing.T) {
	tools := []config.Tool{
		{Name: "no-install", Check: "__devx_definitely_not_a_real_tool_xyz__"},
		// Install block is empty — no command for any platform.
	}
	results := RunTools(context.Background(), tools, Options{Fix: true})
	if len(results) != 1 || results[0].Status != "failed" {
		t.Fatalf("expected status 'failed', got %+v", results)
	}
}

func TestRunTools_EmptyList(t *testing.T) {
	results := RunTools(context.Background(), nil, Options{})
	if len(results) != 0 {
		t.Fatalf("expected empty results for empty tool list")
	}
}

// ── RunSteps ──────────────────────────────────────────────────────────────

func setupTempState(t *testing.T) func() {
	t.Helper()
	dir := t.TempDir()
	orig := stateFile
	stateFile = filepath.Join(dir, ".devx", "setup-state.json")
	return func() { stateFile = orig }
}

func TestRunSteps_BasicSuccess(t *testing.T) {
	defer setupTempState(t)()

	steps := []config.SetupStep{
		{Name: "greet", Run: echoCmd("hello")},
	}
	results := RunSteps(steps, Options{})
	if len(results) != 1 || results[0].Status != "ok" {
		t.Fatalf("expected status 'ok', got %+v", results)
	}
	if results[0].Type != "step" {
		t.Fatalf("expected type 'step', got %q", results[0].Type)
	}
}

func TestRunSteps_FailedStep(t *testing.T) {
	defer setupTempState(t)()

	steps := []config.SetupStep{
		{Name: "fail", Run: alwaysFailCmd()},
	}
	results := RunSteps(steps, Options{})
	if len(results) != 1 || results[0].Status != "failed" {
		t.Fatalf("expected status 'failed', got %+v", results)
	}
}

func TestRunSteps_FailedStep_DoesNotMarkDone(t *testing.T) {
	defer setupTempState(t)()

	step := config.SetupStep{Name: "fail", Run: alwaysFailCmd(), RunOnce: true}
	RunSteps([]config.SetupStep{step}, Options{})

	// Run again — should NOT be skipped because the first run failed.
	results := RunSteps([]config.SetupStep{step}, Options{})
	if results[0].Status == "skipped" {
		t.Fatal("failed step must not be marked as done — should re-run on next invocation")
	}
}

func TestRunSteps_FailedStep_DoesNotBlockSubsequent(t *testing.T) {
	defer setupTempState(t)()

	steps := []config.SetupStep{
		{Name: "fail", Run: alwaysFailCmd()},
		{Name: "ok", Run: echoCmd("after-fail")},
	}
	results := RunSteps(steps, Options{})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Status != "failed" {
		t.Errorf("expected first step to fail, got %q", results[0].Status)
	}
	if results[1].Status != "ok" {
		t.Errorf("expected second step to succeed, got %q", results[1].Status)
	}
}

func TestRunSteps_RunOnce_SkipsAfterSuccess(t *testing.T) {
	defer setupTempState(t)()

	step := config.SetupStep{Name: "restore", Run: echoCmd("install"), RunOnce: true}

	first := RunSteps([]config.SetupStep{step}, Options{})
	if first[0].Status != "ok" {
		t.Fatalf("first run: expected 'ok', got %q", first[0].Status)
	}

	second := RunSteps([]config.SetupStep{step}, Options{})
	if second[0].Status != "skipped" {
		t.Fatalf("second run: expected 'skipped', got %q", second[0].Status)
	}
}

func TestRunSteps_RunOnce_RerunsOnCommandChange(t *testing.T) {
	defer setupTempState(t)()

	stepV1 := config.SetupStep{Name: "restore", Run: echoCmd("v1"), RunOnce: true}
	RunSteps([]config.SetupStep{stepV1}, Options{})

	stepV2 := config.SetupStep{Name: "restore", Run: echoCmd("v2"), RunOnce: true}
	second := RunSteps([]config.SetupStep{stepV2}, Options{})
	if second[0].Status != "ok" {
		t.Fatalf("expected step to re-run after command changed, got %q", second[0].Status)
	}
}

func TestRunSteps_RunOnce_RerunsOnFix(t *testing.T) {
	defer setupTempState(t)()

	step := config.SetupStep{Name: "restore", Run: echoCmd("install"), RunOnce: true}
	RunSteps([]config.SetupStep{step}, Options{})

	results := RunSteps([]config.SetupStep{step}, Options{Fix: true})
	if results[0].Status != "ok" {
		t.Fatalf("expected runOnce step to re-run with --fix, got %q", results[0].Status)
	}
}

func TestRunSteps_PlatformFilter_NotApplicable(t *testing.T) {
	defer setupTempState(t)()

	// Pick a platform that's definitely NOT the current OS.
	var otherPlatform string
	if goruntime.GOOS == "windows" {
		otherPlatform = "linux"
	} else {
		otherPlatform = "windows"
	}

	steps := []config.SetupStep{
		{Name: "platform-step", Run: echoCmd("hello"), Platform: otherPlatform},
	}
	results := RunSteps(steps, Options{})
	if len(results) != 1 || results[0].Status != "not-applicable" {
		t.Fatalf("expected status 'not-applicable', got %+v", results)
	}
}

func TestRunSteps_PlatformFilter_CurrentOS(t *testing.T) {
	defer setupTempState(t)()

	var currentPlatform string
	switch goruntime.GOOS {
	case "windows":
		currentPlatform = "windows"
	case "darwin":
		currentPlatform = "macos"
	default:
		currentPlatform = "linux"
	}

	steps := []config.SetupStep{
		{Name: "platform-step", Run: echoCmd("hello"), Platform: currentPlatform},
	}
	results := RunSteps(steps, Options{})
	if results[0].Status == "not-applicable" {
		t.Fatalf("expected step to run on current OS %q, got 'not-applicable'", goruntime.GOOS)
	}
}

func TestRunSteps_EmptyList(t *testing.T) {
	defer setupTempState(t)()
	results := RunSteps(nil, Options{})
	if len(results) != 0 {
		t.Fatal("expected empty results for empty step list")
	}
}

func TestResolveTools(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, ".nvmrc"), []byte("20.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ResolveTools(d, []config.Tool{
		{Name: "node"},
		{Name: "node", Version: "18"},
	})
	if got[0].Version != "20.1.0" || got[0].Source != ".nvmrc" || got[0].Check != "node --version" {
		t.Errorf("fill: %+v", got[0])
	}
	if got[1].Version != "18" || got[1].Conflict == "" {
		t.Errorf("conflict: %+v", got[1])
	}
}

func TestResolveSteps(t *testing.T) {
	d := t.TempDir()
	if got := ResolveSteps(d, nil); got != nil {
		t.Fatalf("expected none, got %+v", got)
	}
	if err := os.WriteFile(filepath.Join(d, "package.json"), []byte(`{"workspaces":["a"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ResolveSteps(d, nil); len(got) != 1 || got[0].Run != "npm install" {
		t.Fatalf("auto: %+v", got)
	}
	got := ResolveSteps(d, []config.SetupStep{{Name: "i", Workspace: "auto"}})
	if got[0].Run != "npm install" {
		t.Fatalf("explicit: %+v", got)
	}
}

func TestRunEnvFiles(t *testing.T) {
	d := t.TempDir()
	wd, _ := os.Getwd()
	if err := os.Chdir(d); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	t.Setenv("DEVX_TEST_TOKEN", "s3cret")
	if err := os.WriteFile("c.tmpl", []byte("token: ${TOKEN}\nkey: ${KEY}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := config.EnvFile{File: "c.yaml", Template: "c.tmpl", Values: map[string]config.EnvSource{
		"TOKEN": {From: "env", Name: "DEVX_TEST_TOKEN"},
		"KEY":   {From: "prompt", Default: "generate"},
	}}
	r := RunEnvFiles([]config.EnvFile{f}, EnvOptions{})[0]
	if r.Status != "ok" || strings.Contains(r.Detail, "s3cret") {
		t.Fatalf("%+v", r)
	}
	b, _ := os.ReadFile("c.yaml")
	if !strings.Contains(string(b), "token: s3cret") || strings.Contains(string(b), "${") {
		t.Fatalf("body: %s", b)
	}
	if r := RunEnvFiles([]config.EnvFile{f}, EnvOptions{})[0]; r.Status != "skipped" {
		t.Fatalf("expected skipped, got %+v", r)
	}

	g := config.EnvFile{File: "d.env", Values: map[string]config.EnvSource{"NOPE": {From: "env", Name: "DEVX_TEST_UNSET"}}}
	if r := RunEnvFiles([]config.EnvFile{g}, EnvOptions{})[0]; r.Status != "missing" || !strings.Contains(r.Detail, "NOPE") {
		t.Fatalf("expected missing: %+v", r)
	}
}
