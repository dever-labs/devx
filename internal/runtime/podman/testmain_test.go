package podman

import (
	"os"
	"testing"

	"github.com/dever-labs/devx/internal/testexec"
)

// TestMain lets writeFakeBinary's copies of this test binary (see
// testexec.NewFakeBin) re-exec into a fake "podman"/"podman-compose"
// process instead of re-running the whole test suite — see
// internal/testexec's package doc for why.
func TestMain(m *testing.M) {
	if testexec.IsHelperProcess() {
		testexec.RunHelperProcess()
		return // unreachable: RunHelperProcess always calls os.Exit
	}
	os.Exit(m.Run())
}
