package runtime

import "testing"

func TestErrNoRuntime(t *testing.T) {
	if ErrNoRuntime == nil {
		t.Fatalf("ErrNoRuntime is nil")
	}
}
