package main

import (
	"context"
	"testing"
)

func TestRunSetup_MissingManifestErrors(t *testing.T) {
	chdirTemp(t)
	if err := runSetup(context.Background(), nil); err == nil {
		t.Error("expected an error when devx.yaml doesn't exist")
	}
}
