package task

import (
	"runtime"
	"testing"
)

func TestOSMatches_EmptyListAlwaysMatches(t *testing.T) {
	if !osMatches(nil) {
		t.Error("expected an empty os: filter to always match")
	}
	if !osMatches([]string{}) {
		t.Error("expected an empty os: filter to always match")
	}
}

func TestOSMatches_CurrentGOOS(t *testing.T) {
	if !osMatches([]string{runtime.GOOS}) {
		t.Errorf("expected os: [%s] to match the current GOOS", runtime.GOOS)
	}
}

func TestOSMatches_OtherGOOSDoesNotMatch(t *testing.T) {
	bogus := "not-a-real-os"
	if osMatches([]string{bogus}) {
		t.Errorf("expected os: [%s] not to match GOOS %q", bogus, runtime.GOOS)
	}
}

func TestOSMatches_MacOSAliasForDarwin(t *testing.T) {
	matches := osMatches([]string{"macos"})
	wantMatch := runtime.GOOS == "darwin"
	if matches != wantMatch {
		t.Errorf("osMatches([macos]) = %v on GOOS %q, want %v", matches, runtime.GOOS, wantMatch)
	}
}

func TestOSMatches_MultipleEntriesOneMatches(t *testing.T) {
	if !osMatches([]string{"not-a-real-os", runtime.GOOS}) {
		t.Errorf("expected a match when one of several os: entries is the current GOOS")
	}
}
