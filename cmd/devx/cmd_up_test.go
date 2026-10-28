package main

import (
	"context"
	"testing"
)

func TestRunUp_MissingManifestErrors(t *testing.T) {
	chdirTemp(t)
	if err := runUp(context.Background(), []string{"--dry-run"}); err == nil {
		t.Error("expected an error when devx.yaml doesn't exist")
	}
}
