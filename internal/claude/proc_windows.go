//go:build windows

package claude

import (
	"os/exec"
	"strconv"
	"time"
)

func prepare(cmd *exec.Cmd) {}

// killTree ends claude and everything it started.
func killTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run(); err != nil {
		cmd.Process.Kill()
	}
}

// cpuTime is not measured on Windows yet.
func cpuTime(pid int) time.Duration { return -1 }
