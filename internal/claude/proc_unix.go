//go:build !windows

package claude

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// prepare puts claude in a process group of its own, so that killing it also
// kills what it started (hooks and the like).
func prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	// A negative pid signals the whole group.
	syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

// cpuTime returns the processor time pid has used so far, or -1.
func cpuTime(pid int) time.Duration {
	out, err := exec.Command("ps", "-o", "time=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return -1
	}
	return parsePSTime(strings.TrimSpace(string(out)))
}

// parsePSTime reads the "time" column of ps: [[dd-]hh:]mm:ss[.cc].
func parsePSTime(s string) time.Duration {
	if s == "" {
		return -1
	}
	var days time.Duration
	if d, rest, ok := strings.Cut(s, "-"); ok {
		n, err := strconv.Atoi(d)
		if err != nil {
			return -1
		}
		days = time.Duration(n) * 24 * time.Hour
		s = rest
	}
	parts := strings.Split(s, ":")
	secs, err := strconv.ParseFloat(parts[len(parts)-1], 64)
	if err != nil {
		return -1
	}
	total := days + time.Duration(secs*float64(time.Second))
	unit := time.Minute
	for i := len(parts) - 2; i >= 0; i-- {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return -1
		}
		total += time.Duration(n) * unit
		unit *= 60
	}
	return total
}
