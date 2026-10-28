package main

import "testing"

func TestExpandRepoArg(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"dever-labs/devx", "https://github.com/dever-labs/devx"},
		{"https://github.com/dever-labs/devx", "https://github.com/dever-labs/devx"},
		{"http://example.com/repo.git", "http://example.com/repo.git"},
		{"git@github.com:dever-labs/devx.git", "git@github.com:dever-labs/devx.git"},
		{"./local/path", "./local/path"},
	}
	for _, tc := range cases {
		if got := expandRepoArg(tc.in); got != tc.want {
			t.Errorf("expandRepoArg(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRepoBaseName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://github.com/dever-labs/devx.git", "devx"},
		{"https://github.com/dever-labs/devx", "devx"},
		{"git@github.com:dever-labs/devx.git", "devx"},
		{"devx", "devx"},
	}
	for _, tc := range cases {
		if got := repoBaseName(tc.in); got != tc.want {
			t.Errorf("repoBaseName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
