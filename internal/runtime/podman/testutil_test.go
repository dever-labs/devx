package podman

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFakeBinary creates a shell script that records every argument it
// receives (one per line) to the file named by $FAKE_ARGS_FILE, optionally
// emits the contents of $FAKE_STDOUT_FILE to stdout, and exits with
// $FAKE_EXIT_CODE (default 0). This lets tests fully control docker/compose
// behavior without needing the real CLI installed, while still exercising
// the actual exec.Command plumbing in Runtime's methods.
func writeFakeBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fakebin")
	script := `#!/bin/sh
if [ -n "$FAKE_ARGS_FILE" ]; then
  printf '%s\n' "$@" > "$FAKE_ARGS_FILE"
fi
if [ -n "$FAKE_STDOUT_FILE" ]; then
  cat "$FAKE_STDOUT_FILE"
fi
exit "${FAKE_EXIT_CODE:-0}"
`
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return path
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
