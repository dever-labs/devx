package task

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSubstitute_BasicAndEscape(t *testing.T) {
	r := NewResolver(nil, nil, map[string]string{"PORT": "5432"})
	out, err := r.Substitute("postgres on ${PORT}")
	if err != nil {
		t.Fatal(err)
	}
	if out != "postgres on 5432" {
		t.Fatalf("got %q", out)
	}

	out, err = r.Substitute("literal $${PORT} stays")
	if err != nil {
		t.Fatal(err)
	}
	if out != "literal ${PORT} stays" {
		t.Fatalf("got %q", out)
	}
}

func TestSubstitute_UnbracedDollarUntouched(t *testing.T) {
	r := NewResolver(nil, nil, nil)
	out, err := r.Substitute("echo $SECRET")
	if err != nil {
		t.Fatal(err)
	}
	if out != "echo $SECRET" {
		t.Fatalf("expected unbraced $VAR untouched, got %q", out)
	}
}

func TestSubstitute_Precedence(t *testing.T) {
	os.Setenv("DEVX_TEST_VAR", "from-env")
	defer os.Unsetenv("DEVX_TEST_VAR")

	r := NewResolver(
		map[string]string{"DEVX_TEST_VAR": "from-cli"},
		map[string]string{"DEVX_TEST_VAR": "from-dotenv"},
		map[string]string{"DEVX_TEST_VAR": "from-vars"},
	)
	out, _ := r.Substitute("${DEVX_TEST_VAR}")
	if out != "from-cli" {
		t.Fatalf("CLI override should win, got %q", out)
	}

	r2 := NewResolver(nil, map[string]string{"DEVX_TEST_VAR": "from-dotenv"}, map[string]string{"DEVX_TEST_VAR": "from-vars"})
	out2, _ := r2.Substitute("${DEVX_TEST_VAR}")
	if out2 != "from-env" {
		t.Fatalf("inherited env should beat .env/vars, got %q", out2)
	}
}

func TestSubstitute_UndefinedIsError(t *testing.T) {
	r := NewResolver(nil, nil, nil)
	if _, err := r.Substitute("${NOPE}"); err == nil {
		t.Fatalf("expected error for undefined variable")
	}
}

func TestInfer_PortAndName(t *testing.T) {
	shape := Infer("docker run --rm -d --name db -p 5432:5432 postgres:16")
	if shape.Port != 5432 {
		t.Fatalf("expected port 5432, got %d", shape.Port)
	}
	if shape.ContainerName != "db" {
		t.Fatalf("expected name 'db', got %q", shape.ContainerName)
	}
	if !shape.IsContainer {
		t.Fatalf("expected IsContainer true")
	}
	if !shape.Detached {
		t.Fatalf("expected Detached true (port + -d)")
	}
}

func TestInfer_OneShotNoPort(t *testing.T) {
	shape := Infer("./scripts/migrate.sh up")
	if shape.Port != 0 || shape.Detached {
		t.Fatalf("expected one-shot shape, got %+v", shape)
	}
}

func TestUpToDate_SkipsWhenGeneratedNewer(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.go")
	out := filepath.Join(dir, "bin")
	if err := os.WriteFile(src, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}
	// Ensure out is strictly newer than src.
	now := time.Now()
	os.Chtimes(src, now.Add(-time.Minute), now.Add(-time.Minute))
	os.Chtimes(out, now, now)

	et := &EffectiveTask{ResolvedDir: dir}
	et.Sources = []string{"src.go"}
	et.Generates = []string{"bin"}
	if !upToDate(et) {
		t.Fatalf("expected up to date")
	}

	// Now make src newer than out: should rebuild.
	os.Chtimes(src, now.Add(time.Minute), now.Add(time.Minute))
	if upToDate(et) {
		t.Fatalf("expected stale (src newer than generated)")
	}
}
