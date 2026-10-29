package localai

import (
	"runtime"
	"strings"
	"testing"
)

func TestSetup_UnknownBackendErrors(t *testing.T) {
	err := Setup(SetupOptions{Backend: "bogus-backend"})
	if err == nil {
		t.Fatal("expected an error for an unknown backend")
	}
	if !strings.Contains(err.Error(), "unknown backend") {
		t.Errorf("error = %q, want it to mention 'unknown backend'", err)
	}
}

func TestSetup_MLXOnNonAppleSiliconErrors(t *testing.T) {
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		t.Skip("this test asserts MLX setup fails on non-Apple-Silicon hosts")
	}
	err := Setup(SetupOptions{Backend: BackendMLX})
	if err == nil {
		t.Fatal("expected an error requesting MLX setup on a non-Apple-Silicon host")
	}
	if !strings.Contains(err.Error(), "Apple Silicon") {
		t.Errorf("error = %q, want it to mention Apple Silicon", err)
	}
}

func TestJoinArgs_QuotesEachArgument(t *testing.T) {
	got := joinArgs([]string{"foo", "bar baz"})
	want := `"foo" "bar baz"`
	if got != want {
		t.Errorf("joinArgs() = %q, want %q", got, want)
	}
}

func TestJoinArgs_EmptyReturnsEmptyString(t *testing.T) {
	if got := joinArgs(nil); got != "" {
		t.Errorf("joinArgs(nil) = %q, want empty string", got)
	}
}
