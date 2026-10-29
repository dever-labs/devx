package podman

import (
	"os"
	"strings"
	"testing"

	"github.com/dever-labs/devx/internal/testexec"
)

// writeFakeBinary returns a cross-platform fake binary (see
// internal/testexec) that records every argument it receives (one per
// line) to the file named by $FAKE_ARGS_FILE, optionally emits the
// contents of $FAKE_STDOUT_FILE to stdout, and exits with
// $FAKE_EXIT_CODE (default 0). This lets tests fully control
// podman/compose behavior without needing the real CLI installed, while
// still exercising the actual exec.Command plumbing in Runtime's methods.
func writeFakeBinary(t *testing.T) string {
	t.Helper()
	return testexec.NewFakeBin(t)
}

func readArgsFile(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading args file: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
