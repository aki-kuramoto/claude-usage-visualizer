//go:build windows

package job

import (
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Job is a job object holding a process and what it starts. Its methods may
// be called from any goroutine; after Close they do nothing.
type Job struct {
	mu sync.Mutex
	h  windows.Handle
}

// New makes an empty job. Should this application end without closing it,
// Windows closes it at exit, which ends the processes in it too.
func New() (*Job, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	return &Job{h: h}, nil
}

// Add puts the process with the given handle in the job. Processes it
// starts from then on are in the job too.
func (j *Job) Add(process windows.Handle) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.h == 0 {
		return windows.ERROR_INVALID_HANDLE
	}
	return windows.AssignProcessToJobObject(j.h, process)
}

// AddPID is Add for a process known by its id.
func (j *Job) AddPID(pid int) error {
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return j.Add(h)
}

// Terminate ends every process in the job.
func (j *Job) Terminate() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.h == 0 {
		return windows.ERROR_INVALID_HANDLE
	}
	return windows.TerminateJobObject(j.h, 1)
}

// basicAccounting is JOBOBJECT_BASIC_ACCOUNTING_INFORMATION.
type basicAccounting struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

// CPU returns the processor time every process ever in the job has used,
// or -1.
func (j *Job) CPU() time.Duration {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.h == 0 {
		return -1
	}
	var info basicAccounting
	if err := windows.QueryInformationJobObject(j.h, windows.JobObjectBasicAccountingInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil); err != nil {
		return -1
	}
	// The times are in units of 100 nanoseconds.
	return time.Duration(info.TotalUserTime+info.TotalKernelTime) * 100
}

// Close releases the job, ending what is still running in it.
func (j *Job) Close() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.h != 0 {
		windows.CloseHandle(j.h)
		j.h = 0
	}
}
