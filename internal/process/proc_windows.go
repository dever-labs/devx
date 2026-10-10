//go:build windows

package process

import (
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
	processQueryLimited   = 0x1000
	stillActive           = 259
)

func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | detachedProcess, HideWindow: true}
}

func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := syscall.OpenProcess(processQueryLimited, false, uint32(pid))
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

// terminate kills the whole process tree; taskkill /T is the reliable way to
// reach children of shell wrappers such as `cmd /C yarn dev`.
func terminate(pid int, _ time.Duration) {
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}
