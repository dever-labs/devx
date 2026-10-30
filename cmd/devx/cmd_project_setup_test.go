package main

import (
	"context"
	"testing"
)

func TestRunProjectSetup_MissingManifestErrors(t *testing.T) {
	chdirTemp(t)
	if err := runProjectSetup(context.Background(), nil); err == nil {
		t.Error("expected an error when devx.yaml doesn't exist")
	}
}
