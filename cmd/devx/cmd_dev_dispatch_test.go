package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunDev_NoArgsShowsStatus(t *testing.T) {
	chdirTemp(t)
	if err := runDev(context.Background(), nil); err == nil {
		t.Error("expected an error (no devcontainer.json) when dispatching to status with no args")
	}
}

func TestRunDev_HelpPrintsUsage(t *testing.T) {
	var err error
	out := captureStdout(t, func() {
		err = runDev(context.Background(), []string{"help"})
	})
	if err != nil {
		t.Fatalf("runDev(help) failed: %v", err)
	}
	if !strings.Contains(out, "devx dev") {
		t.Errorf("expected usage output, got:\n%s", out)
	}
}

func TestRunDev_UnknownSubcommandErrors(t *testing.T) {
	var err error
	_ = captureStdout(t, func() {
		err = runDev(context.Background(), []string{"bogus"})
	})
	if err == nil || !strings.Contains(err.Error(), "unknown dev subcommand") {
		t.Errorf("runDev(bogus) = %v, want 'unknown dev subcommand' error", err)
	}
}

func TestRunDevBuild_NoDevcontainerCLIErrors(t *testing.T) {
	chdirTemp(t)
	if err := runDevBuild(nil, false); err == nil {
		t.Skip("devcontainer CLI appears to be installed in this environment")
	}
}

func TestRunDevUp_NoDevcontainerCLIErrors(t *testing.T) {
	chdirTemp(t)
	if err := runDevUp(nil); err == nil {
		t.Skip("devcontainer CLI appears to be installed in this environment")
	}
}

func TestRunDevExec_NoArgsErrors(t *testing.T) {
	if err := runDevExec(nil); err == nil {
		t.Error("expected an error when no command is given")
	}
}

func TestRunDevExec_NoDevcontainerCLIErrors(t *testing.T) {
	chdirTemp(t)
	if err := runDevExec([]string{"ls"}); err == nil {
		t.Skip("devcontainer CLI appears to be installed in this environment")
	}
}

func TestRunDevOpen_NoDevcontainerErrors(t *testing.T) {
	chdirTemp(t)
	if err := runDevOpen(); err == nil {
		t.Error("expected an error when no devcontainer.json exists")
	}
}

func TestRunDevPS_ListsOrReportsNone(t *testing.T) {
	var err error
	out := captureStdout(t, func() {
		err = runDevPS()
	})
	if err != nil {
		t.Fatalf("runDevPS failed: %v", err)
	}
	if out == "" {
		t.Error("expected some output from runDevPS")
	}
}
