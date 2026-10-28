//go:build windows

package task

import (
	"fmt"
	"os/exec"
	"syscall"
)

// setPgid is a no-op on Windows — there's no process-group equivalent to
// set up at process-start time; task-tree termination instead uses
// taskkill /T below, which walks the live process tree at stop time.
func setPgid(attr *syscall.SysProcAttr) {}

// stopProcessTree asks Windows to terminate pid's process tree gracefully
// (no /F — requests a clean WM_CLOSE-style shutdown first).
func stopProcessTree(pid int) {
	_ = exec.Command("taskkill", "/T", "/PID", fmt.Sprint(pid)).Run()
}

// killProcessTree force-terminates pid's entire process tree.
func killProcessTree(pid int) {
	_ = exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprint(pid)).Run()
}

// processAliveOS reports whether pid is still running. Windows has no
// signal-0 liveness probe, so this uses OpenProcess + GetExitCodeProcess
// instead and checks for the STILL_ACTIVE sentinel exit code.
func processAliveOS(pid int) bool {
	const stillActive = 259
	h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)

	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}
