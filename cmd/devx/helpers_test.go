package main

import (
	"context"
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	devxruntime "github.com/dever-labs/devx/internal/runtime"
	"github.com/dever-labs/devx/internal/task"
)

const validManifest = `version: 1
project:
  name: my-app
api:
  run: "echo hi"
`

// chdirTemp chdirs into a fresh temp dir for the duration of the test,
// restoring the original working directory on cleanup. Equivalent to
// testing.T.Chdir (added in Go 1.24), reimplemented manually since this
// module's go.mod still declares go 1.21.
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

func writeManifest(t *testing.T, content string) {
	t.Helper()
	chdirTemp(t)
	if err := os.WriteFile(manifestFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestParseCLIVars(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantNames []string
		wantVars  map[string]string
	}{
		{"no args", nil, nil, map[string]string{}},
		{"names only", []string{"api", "worker"}, []string{"api", "worker"}, map[string]string{}},
		{"vars only", []string{"PORT=8080", "ENV=dev"}, nil, map[string]string{"PORT": "8080", "ENV": "dev"}},
		{"mixed", []string{"api", "PORT=8080"}, []string{"api"}, map[string]string{"PORT": "8080"}},
		{"path-like value is not mistaken for a var", []string{"./api", "../worker"}, []string{"./api", "../worker"}, map[string]string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			names, vars := parseCLIVars(tc.args)
			if !reflect.DeepEqual(names, tc.wantNames) {
				t.Errorf("names = %v, want %v", names, tc.wantNames)
			}
			if !reflect.DeepEqual(vars, tc.wantVars) {
				t.Errorf("vars = %v, want %v", vars, tc.wantVars)
			}
		})
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "present")
	if err := os.WriteFile(present, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if !fileExists(present) {
		t.Error("expected fileExists to be true for an existing file")
	}
	if fileExists(filepath.Join(dir, "missing")) {
		t.Error("expected fileExists to be false for a missing file")
	}
}

func TestEnsureDevxDir(t *testing.T) {
	chdirTemp(t)
	if err := ensureDevxDir(); err != nil {
		t.Fatalf("ensureDevxDir: %v", err)
	}
	info, err := os.Stat(devxDir)
	if err != nil {
		t.Fatalf("expected %s to exist: %v", devxDir, err)
	}
	if !info.IsDir() {
		t.Errorf("expected %s to be a directory", devxDir)
	}
}

func TestEnsureGitignore(t *testing.T) {
	t.Run("creates .gitignore when missing", func(t *testing.T) {
		chdirTemp(t)
		if err := ensureGitignore(); err != nil {
			t.Fatalf("ensureGitignore: %v", err)
		}
		data, err := os.ReadFile(".gitignore")
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != ".devx/\n" {
			t.Errorf(".gitignore = %q, want %q", data, ".devx/\n")
		}
	})

	t.Run("appends entry to existing .gitignore", func(t *testing.T) {
		chdirTemp(t)
		if err := os.WriteFile(".gitignore", []byte("node_modules/\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := ensureGitignore(); err != nil {
			t.Fatalf("ensureGitignore: %v", err)
		}
		data, err := os.ReadFile(".gitignore")
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "node_modules/\n\n.devx/\n" {
			t.Errorf(".gitignore = %q", data)
		}
	})

	t.Run("is idempotent when entry already present", func(t *testing.T) {
		chdirTemp(t)
		if err := os.WriteFile(".gitignore", []byte(".devx/\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := ensureGitignore(); err != nil {
			t.Fatalf("ensureGitignore: %v", err)
		}
		data, err := os.ReadFile(".gitignore")
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != ".devx/\n" {
			t.Errorf(".gitignore = %q, want unchanged %q", data, ".devx/\n")
		}
	})
}

func TestStateFilePathAndLogDirPath(t *testing.T) {
	if got, want := stateFilePath(), filepath.Join(devxDir, "tasks.json"); got != want {
		t.Errorf("stateFilePath() = %q, want %q", got, want)
	}
	if got, want := logDirPath(), filepath.Join(devxDir, "logs"); got != want {
		t.Errorf("logDirPath() = %q, want %q", got, want)
	}
}

func TestLoadManifest(t *testing.T) {
	t.Run("valid manifest", func(t *testing.T) {
		writeManifest(t, validManifest)
		m, err := loadManifest()
		if err != nil {
			t.Fatalf("loadManifest: %v", err)
		}
		if m.Project.Name != "my-app" {
			t.Errorf("project name = %q, want my-app", m.Project.Name)
		}
	})

	t.Run("missing manifest", func(t *testing.T) {
		chdirTemp(t)
		if _, err := loadManifest(); err == nil {
			t.Fatal("expected error for missing devx.yaml")
		}
	})

	t.Run("malformed yaml", func(t *testing.T) {
		writeManifest(t, "not: valid: yaml: [")
		if _, err := loadManifest(); err == nil {
			t.Fatal("expected error for malformed devx.yaml")
		}
	})
}

func TestLoadValidManifest(t *testing.T) {
	t.Run("passes validation", func(t *testing.T) {
		writeManifest(t, validManifest)
		if _, err := loadValidManifest(); err != nil {
			t.Fatalf("loadValidManifest: %v", err)
		}
	})

	t.Run("fails validation when a task has no run", func(t *testing.T) {
		writeManifest(t, `version: 1
project:
  name: my-app
api: {}
`)
		if _, err := loadValidManifest(); err == nil {
			t.Fatal("expected validation error for a task missing run:")
		}
	})
}

func TestNewScheduler(t *testing.T) {
	writeManifest(t, validManifest)
	m, err := loadValidManifest()
	if err != nil {
		t.Fatalf("loadValidManifest: %v", err)
	}
	sched, err := newScheduler(m, map[string]string{"FOO": "bar"}, task.Options{})
	if err != nil {
		t.Fatalf("newScheduler: %v", err)
	}
	if sched == nil {
		t.Fatal("expected non-nil scheduler")
	}
}

func TestLookPathQuiet(t *testing.T) {
	if !lookPathQuiet("sh") {
		t.Error("expected lookPathQuiet(\"sh\") to be true (sh should be on PATH)")
	}
	if lookPathQuiet("definitely-not-a-real-binary-xyz") {
		t.Error("expected lookPathQuiet to return false for a nonexistent binary")
	}
}

func TestParseFlagsAnywhere(t *testing.T) {
	newFS := func() (*flag.FlagSet, *bool, *string) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		dryRun := fs.Bool("dry-run", false, "")
		group := fs.String("group", "", "")
		return fs, dryRun, group
	}

	cases := []struct {
		name           string
		args           []string
		wantPositional []string
		wantDryRun     bool
		wantGroup      string
	}{
		{
			name:           "flags before task name",
			args:           []string{"--dry-run", "api"},
			wantPositional: []string{"api"},
			wantDryRun:     true,
		},
		{
			name:           "flags after task name (issue #59)",
			args:           []string{"api", "--dry-run"},
			wantPositional: []string{"api"},
			wantDryRun:     true,
		},
		{
			name:           "value flag after task name",
			args:           []string{"api", "--group", "backend"},
			wantPositional: []string{"api"},
			wantGroup:      "backend",
		},
		{
			name:           "value flag with = after task name",
			args:           []string{"api", "--group=backend"},
			wantPositional: []string{"api"},
			wantGroup:      "backend",
		},
		{
			name:           "multiple task names around flags",
			args:           []string{"api", "--dry-run", "worker"},
			wantPositional: []string{"api", "worker"},
			wantDryRun:     true,
		},
		{
			name:           "double-dash separator",
			args:           []string{"--dry-run", "--", "--looks-like-a-flag"},
			wantPositional: []string{"--looks-like-a-flag"},
			wantDryRun:     true,
		},
		{
			name:           "no flags",
			args:           []string{"api", "worker"},
			wantPositional: []string{"api", "worker"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs, dryRun, group := newFS()
			positional, err := parseFlagsAnywhere(fs, tc.args)
			if err != nil {
				t.Fatalf("parseFlagsAnywhere(%v) error: %v", tc.args, err)
			}
			if !reflect.DeepEqual(positional, tc.wantPositional) {
				t.Errorf("positional = %v, want %v", positional, tc.wantPositional)
			}
			if *dryRun != tc.wantDryRun {
				t.Errorf("dryRun = %v, want %v", *dryRun, tc.wantDryRun)
			}
			if *group != tc.wantGroup {
				t.Errorf("group = %q, want %q", *group, tc.wantGroup)
			}
		})
	}
}

func TestParseFlagsAnywhere_UnknownFlagErrors(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	_, err := parseFlagsAnywhere(fs, []string{"api", "--not-a-real-flag"})
	if err == nil {
		t.Fatal("expected an error for an unknown flag")
	}
}

func TestSelectContainerRuntime_ReturnsRuntimeOrErrNoRuntime(t *testing.T) {
	rt, err := selectContainerRuntime(context.Background())
	if err != nil {
		if err != devxruntime.ErrNoRuntime {
			t.Errorf("expected ErrNoRuntime or a valid runtime, got error: %v", err)
		}
		return
	}
	if rt == nil {
		t.Error("expected a non-nil runtime when err is nil")
	}
}
