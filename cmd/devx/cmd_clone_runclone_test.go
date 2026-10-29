package main

import (
	"context"
	"testing"
)

func TestRunClone_NoArgsErrors(t *testing.T) {
	if err := runClone(context.Background(), nil); err == nil {
		t.Error("expected an error when no repo argument is given")
	}
}
