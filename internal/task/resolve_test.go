package task

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dever-labs/devx/internal/config"
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

// TestInfer_HostPortWhenDifferentFromContainerPort guards against a
// regression where the regex captured the container-internal port
// instead of the host-reachable one whenever they differ.
func TestInfer_HostPortWhenDifferentFromContainerPort(t *testing.T) {
	cases := map[string]int{
		"docker run -p 8080:80 nginx":           8080,
		"docker run -p 5432:5432 postgres":      5432,
		"docker run -p 127.0.0.1:8080:80 nginx": 8080,
		"docker run -p 8080:80/tcp nginx":       8080,
		"docker run --port 3000 node":           3000,
	}
	for run, want := range cases {
		if got := Infer(run).Port; got != want {
			t.Errorf("Infer(%q).Port = %d, want %d", run, got, want)
		}
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

func TestRedactedEnv_RedactsAllValues(t *testing.T) {
	out := RedactedEnv(map[string]string{"API_KEY": "super-secret", "DEBUG": "true"})
	if len(out) != 2 {
		t.Fatalf("expected 2 entries, got %+v", out)
	}
	if out["API_KEY"] != "<redacted>" || out["DEBUG"] != "<redacted>" {
		t.Errorf("RedactedEnv() = %+v, want all values redacted", out)
	}
}

func TestRedactedEnv_EmptyMap(t *testing.T) {
	out := RedactedEnv(map[string]string{})
	if len(out) != 0 {
		t.Errorf("RedactedEnv(empty) = %+v, want empty map", out)
	}
}

// TestResolve_DescSubstitution is a regression test for issue #60: a
// ${VAR} reference inside desc: was never passed through the resolver,
// so it showed up literally in `devx tasks` output instead of being
// substituted like run:/env:/dir: are.
func TestResolve_DescSubstitution(t *testing.T) {
	r := NewResolver(nil, nil, map[string]string{"ENVIRONMENT": "staging"})
	et, err := Resolve(config.Task{Name: "api", Run: "echo hi", Desc: "Deploys to ${ENVIRONMENT}"}, r)
	if err != nil {
		t.Fatal(err)
	}
	if et.ResolvedDesc != "Deploys to staging" {
		t.Errorf("ResolvedDesc = %q, want %q", et.ResolvedDesc, "Deploys to staging")
	}
}

// TestResolve_DescSubstitution_UndefinedVarFallsBack confirms that an
// undefined variable in desc: falls back to the raw text instead of
// failing resolution entirely — desc: is purely informational, so it
// shouldn't be able to break `devx up`/`devx tasks` the way a bad run:/
// env: substitution legitimately should.
func TestResolve_DescSubstitution_UndefinedVarFallsBack(t *testing.T) {
	r := NewResolver(nil, nil, nil)
	et, err := Resolve(config.Task{Name: "api", Run: "echo hi", Desc: "Uses ${UNDEFINED_VAR}"}, r)
	if err != nil {
		t.Fatalf("Resolve should not fail on an undefined var in desc:, got %v", err)
	}
	if et.ResolvedDesc != "Uses ${UNDEFINED_VAR}" {
		t.Errorf("ResolvedDesc = %q, want raw desc text preserved as fallback", et.ResolvedDesc)
	}
}
