//go:build !windows

package task

import "syscall"

// setPgid configures attr to start the process in its own process group, so
// stopProcessTree/killProcessTree can signal the whole tree at once via a
// negative PID. Windows has no process-group concept; see driver_windows.go.
func setPgid(attr *syscall.SysProcAttr) {
	attr.Setpgid = true
}

// stopProcessTree sends SIGTERM to pid's entire process group.
func stopProcessTree(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
}

// killProcessTree sends SIGKILL to pid's entire process group.
func killProcessTree(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

// processAliveOS reports whether pid refers to a live process, using
// signal 0 (no-op) which only checks existence/permission.
func processAliveOS(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
