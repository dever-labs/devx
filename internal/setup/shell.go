package setup

import (
	"os"
	"os/exec"
	goruntime "runtime"
)

func shellCmd(command string) *exec.Cmd {
	if goruntime.GOOS == "windows" {
		return exec.Command("cmd", "/c", command)
	}
	return exec.Command("sh", "-c", command)
}

// setEcho toggles terminal echo for secret prompts (best effort, unix only).
func setEcho(on bool) {
	if goruntime.GOOS == "windows" {
		return
	}
	arg := "-echo"
	if on {
		arg = "echo"
	}
	c := exec.Command("stty", arg)
	c.Stdin = os.Stdin
	_ = c.Run()
}
