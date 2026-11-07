//go:build windows

package claude

import (
	"os/exec"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// procSys holds the job object claude runs in. A job collects claude and
// everything it starts, which Windows does not otherwise group: the job
// measures their processor time together and ends them together. When claude
// is a .cmd script (as npm installs it), the process started is cmd.exe and
// the real work happens in its child, so the job is what makes both work.
//
// The job is closed when claude exits, on another goroutine than the one that
// reads the time, so mu guards it.
type procSys struct {
	mu  sync.Mutex
	job windows.Handle
}

func prepare(cmd *exec.Cmd) {
	// No console window for claude when the application has none.
	cmd.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}

// started puts the new process in a job of its own. Children it started
// before this are not in the job; claude starts none that early.
func started(p *process) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	// Should this application end without stopping claude, closing the
	// job's last handle (which Windows does at exit) ends claude too.
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(p.pid()))
	if err != nil {
		windows.CloseHandle(job)
		return
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		windows.CloseHandle(job)
		return
	}
	p.sys.mu.Lock()
	p.sys.job = job
	p.sys.mu.Unlock()
}

// exited releases the job once claude has ended.
func exited(p *process) {
	p.sys.mu.Lock()
	defer p.sys.mu.Unlock()
	if p.sys.job != 0 {
		windows.CloseHandle(p.sys.job)
		p.sys.job = 0
	}
}

// killTree ends claude and everything it started.
func killTree(p *process) {
	if p.cmd.Process == nil {
		return
	}
	p.sys.mu.Lock()
	done := p.sys.job != 0 && windows.TerminateJobObject(p.sys.job, 1) == nil
	p.sys.mu.Unlock()
	if done {
		return
	}
	if err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(p.pid())).Run(); err != nil {
		p.cmd.Process.Kill()
	}
}

// jobBasicAccounting is JOBOBJECT_BASIC_ACCOUNTING_INFORMATION.
type jobBasicAccounting struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

// cpuTime returns the processor time claude and its children have used so
// far, or -1.
func cpuTime(p *process) time.Duration {
	p.sys.mu.Lock()
	defer p.sys.mu.Unlock()
	if p.sys.job == 0 {
		return -1
	}
	var info jobBasicAccounting
	if err := windows.QueryInformationJobObject(p.sys.job, windows.JobObjectBasicAccountingInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil); err != nil {
		return -1
	}
	// The times are in units of 100 nanoseconds.
	return time.Duration(info.TotalUserTime+info.TotalKernelTime) * 100
}
