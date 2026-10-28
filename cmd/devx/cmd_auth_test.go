package main

import "testing"

func TestStatusIcon(t *testing.T) {
	if got := statusIcon(true); got != "✓" {
		t.Errorf("statusIcon(true) = %q, want ✓", got)
	}
	if got := statusIcon(false); got != "✗" {
		t.Errorf("statusIcon(false) = %q, want ✗", got)
	}
}
