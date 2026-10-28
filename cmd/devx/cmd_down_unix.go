//go:build !windows

package main

import "syscall"

// stopProcessGroupOS sends SIGTERM to pid's process group (see
// processDriver's Setpgid: true, which creates that group). Best-effort —
// a no-op if the group has already exited (ESRCH).
func stopProcessGroupOS(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
}

// killProcessGroupOS sends SIGKILL to pid's process group.
func killProcessGroupOS(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

// processAliveOS reports whether pid refers to a live process.
func processAliveOS(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
