package main

import "testing"

func TestSetupStatusIcon(t *testing.T) {
	cases := []struct {
		status string
		want   string
	}{
		{"ok", "✓"},
		{"installed", "✓"},
		{"skipped", "–"},
		{"not-applicable", "–"},
		{"failed", "✗"},
		{"missing", "✗"},
		{"other", "?"},
		{"", "?"},
	}
	for _, tc := range cases {
		if got := setupStatusIcon(tc.status); got != tc.want {
			t.Errorf("setupStatusIcon(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}
