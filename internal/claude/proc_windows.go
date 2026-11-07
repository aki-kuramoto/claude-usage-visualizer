//go:build windows

package claude

import (
	"os/exec"
	"strconv"
	"time"

	"golang.org/x/sys/windows"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/job"
)

// procSys holds the job claude runs in (see package job). When claude is a
// .cmd script (as npm installs it), the process started is cmd.exe and the
// real work happens in its child, so the job is what lets both be measured
// and ended.
type procSys struct {
	job *job.Job
}

func prepare(cmd *exec.Cmd) {
	// No console window for claude when the application has none.
	cmd.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}

// started puts the new process in a job of its own. Children it started
// before this are not in the job; claude starts none that early.
func started(p *process) {
	j, err := job.New()
	if err != nil {
		return
	}
	if err := j.AddPID(p.pid()); err != nil {
		j.Close()
		return
	}
	p.sys.job = j
}

// exited releases the job once claude has ended.
func exited(p *process) {
	if p.sys.job != nil {
		p.sys.job.Close()
	}
}

// killTree ends claude and everything it started.
func killTree(p *process) {
	if p.cmd.Process == nil {
		return
	}
	if p.sys.job != nil && p.sys.job.Terminate() == nil {
		return
	}
	if err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(p.pid())).Run(); err != nil {
		p.cmd.Process.Kill()
	}
}

// cpuTime returns the processor time claude and its children have used so
// far, or -1.
func cpuTime(p *process) time.Duration {
	if p.sys.job == nil {
		return -1
	}
	return p.sys.job.CPU()
}
