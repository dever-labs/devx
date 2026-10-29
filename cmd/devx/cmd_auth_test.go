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

func TestCheckEnvKey_NotSet(t *testing.T) {
	check := checkEnvKey("DEVX_TEST_UNSET_KEY_DOES_NOT_EXIST", "https://example.com/keys")
	ok, hint := check()
	if ok {
		t.Error("expected checkEnvKey to report false for an unset variable")
	}
	if hint == "" {
		t.Error("expected a non-empty hint for an unset variable")
	}
}

func TestCheckEnvKey_Set(t *testing.T) {
	t.Setenv("DEVX_TEST_KEY", "supersecretvalue123")
	check := checkEnvKey("DEVX_TEST_KEY", "")
	ok, hint := check()
	if !ok {
		t.Error("expected checkEnvKey to report true for a set variable")
	}
	if hint == "" {
		t.Error("expected a non-empty hint (masked value) for a set variable")
	}
	if hint == "DEVX_TEST_KEY=supersecretvalue123" {
		t.Error("expected the value in the hint to be masked, not shown in full")
	}
}
