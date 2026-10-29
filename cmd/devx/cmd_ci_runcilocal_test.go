package main

import (
	"strings"
	"testing"
)

func TestRunCILocal_NoActInstalledPrintsInstructions(t *testing.T) {
	var err error
	out := captureStdout(t, func() {
		err = runCILocal(nil)
	})
	if err != nil {
		t.Skip("'act' appears to be installed in this environment")
	}
	if !strings.Contains(out, "act") {
		t.Errorf("expected install instructions mentioning 'act', got:\n%s", out)
	}
}
