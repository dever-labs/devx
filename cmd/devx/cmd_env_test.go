package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceLabel(t *testing.T) {
	dc := map[string]string{"A": "1"}
	dot := map[string]string{"B": "2", "A": "1"}

	cases := []struct {
		key  string
		want string
	}{
		{"A", "dc + .env"},
		{"B", ".env"},
		{"C", "env"},
	}
	for _, tc := range cases {
		if got := sourceLabel(tc.key, dc, dot); got != tc.want {
			t.Errorf("sourceLabel(%q) = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestIsLikelySecret(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"API_KEY", true},
		{"DB_PASSWORD", true},
		{"AUTH_TOKEN", true},
		{"SECRET_VALUE", true},
		{"PORT", false},
		{"HOST", false},
		{"NODE_ENV", false},
	}
	for _, tc := range cases {
		if got := isLikelySecret(tc.key); got != tc.want {
			t.Errorf("isLikelySecret(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestMaskSecret(t *testing.T) {
	cases := []struct {
		val  string
		want string
	}{
		{"", ""},
		{"abc", "***"},
		{"abcdef", "******"},
		{"abcdefgh", "abc**fgh"},
		{"supersecretvalue", "sup**********lue"},
	}
	for _, tc := range cases {
		if got := maskSecret(tc.val); got != tc.want {
			t.Errorf("maskSecret(%q) = %q, want %q", tc.val, got, tc.want)
		}
	}
}

func TestLoadDotEnvToMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# a comment\nFOO=bar\n\nBAZ=\"quoted value\"\nQUX='single quoted'\nMALFORMED-LINE\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	got := loadDotEnvToMap(path)
	want := map[string]string{
		"FOO": "bar",
		"BAZ": "quoted value",
		"QUX": "single quoted",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("loadDotEnvToMap()[%q] = %q, want %q", k, got[k], v)
		}
	}
	if _, ok := got["MALFORMED-LINE"]; ok {
		t.Error("expected a line with no '=' to be skipped")
	}
}

func TestLoadDotEnvToMap_MissingFileReturnsEmpty(t *testing.T) {
	got := loadDotEnvToMap(filepath.Join(t.TempDir(), "does-not-exist.env"))
	if len(got) != 0 {
		t.Errorf("expected empty map for a missing .env file, got %v", got)
	}
}

func TestDotEnvPath(t *testing.T) {
	if got := dotEnvPath(""); got != ".env" {
		t.Errorf("dotEnvPath(\"\") = %q, want %q", got, ".env")
	}

	dir := t.TempDir()
	want := filepath.Join(dir, ".env")
	if got := dotEnvPath(dir); got != want {
		t.Errorf("dotEnvPath(%q) = %q, want %q", dir, got, want)
	}
}
