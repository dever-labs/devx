//go:build !windows

package process

import (
	"os/exec"
	"syscall"
	"time"
)

// detach puts the child in its own session so it survives devx exiting and
// can be signalled as a group.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err != nil && err != syscall.EPERM {
		return false
	}
	// Reap-less zombies still answer signal 0; treat them as dead.
	return !isZombie(pid)
}

// terminate signals the whole process group. Only session leaders created by
// detach are group-signalled, which guards against PID reuse hitting an
// unrelated process.
func terminate(pid int, grace time.Duration) {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return
	}
	target := pid
	if pgid == pid {
		target = -pid
	}
	_ = syscall.Kill(target, syscall.SIGTERM)
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if !alive(pid) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = syscall.Kill(target, syscall.SIGKILL)
}
