//go:build windows

package expiry

import (
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/job"
)

// winTerminal is claude on a pseudo console (ConPTY), the Windows pseudo
// terminal, which Windows 10 1809 and later have. Claude Code needs a newer
// Windows than that anyway.
type winTerminal struct {
	*os.File // what claude draws, as escape sequences
	input    windows.Handle
	console  windows.Handle
	process  windows.Handle
	job      *job.Job
}

func start(path, dir string, env []string) (terminal, error) {
	// Two pipes: one carries what is typed to the pseudo console (nothing
	// is), the other what it draws.
	var inRead, inWrite, outRead, outWrite windows.Handle
	if err := windows.CreatePipe(&inRead, &inWrite, nil, 0); err != nil {
		return nil, err
	}
	if err := windows.CreatePipe(&outRead, &outWrite, nil, 0); err != nil {
		windows.CloseHandle(inRead)
		windows.CloseHandle(inWrite)
		return nil, err
	}
	var console windows.Handle
	err := windows.CreatePseudoConsole(windows.Coord{X: 120, Y: 40}, inRead, outWrite, 0, &console)
	// The pseudo console holds its own references to these ends now.
	windows.CloseHandle(inRead)
	windows.CloseHandle(outWrite)
	if err != nil {
		windows.CloseHandle(inWrite)
		windows.CloseHandle(outRead)
		if strings.Contains(err.Error(), "CreatePseudoConsole") || err == windows.ERROR_PROC_NOT_FOUND {
			return nil, errUnsupported // Windows before 1809
		}
		return nil, err
	}
	t := &winTerminal{File: os.NewFile(uintptr(outRead), "conpty"), input: inWrite, console: console}
	if err := t.startProcess(path, dir, env); err != nil {
		t.stop()
		return nil, err
	}
	return t, nil
}

// startProcess starts claude attached to the pseudo console, inside a job
// so that everything it starts can be ended with it.
func (t *winTerminal) startProcess(path, dir string, env []string) error {
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return err
	}
	defer attrs.Delete()
	// The attribute's value is the pseudo console's handle itself, not a
	// pointer to it.
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		*(*unsafe.Pointer)(unsafe.Pointer(&t.console)), unsafe.Sizeof(t.console)); err != nil {
		return err
	}
	si := &windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.Cb = uint32(unsafe.Sizeof(*si))
	// Invalid standard handles, so that claude gets the pseudo console's
	// rather than this process's own when they are redirected (as they are
	// under go test, or with -headless 2>file); with none given, Windows
	// would hand on this process's.
	si.Flags = windows.STARTF_USESTDHANDLES
	si.StdInput = windows.InvalidHandle
	si.StdOutput = windows.InvalidHandle
	si.StdErr = windows.InvalidHandle

	app, cmdLine := commandLine(path)
	appPtr, err := windows.UTF16PtrFromString(app)
	if err != nil {
		return err
	}
	cmdPtr, err := windows.UTF16PtrFromString(cmdLine)
	if err != nil {
		return err
	}
	dirPtr, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	block := envBlock(env)
	var pi windows.ProcessInformation
	// Suspended until it is in the job, so that nothing it starts escapes.
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_SUSPENDED)
	if err := windows.CreateProcess(appPtr, cmdPtr, nil, nil, false, flags,
		&block[0], dirPtr, &si.StartupInfo, &pi); err != nil {
		return err
	}
	t.process = pi.Process
	if j, err := job.New(); err == nil {
		if j.Add(pi.Process) == nil {
			t.job = j
		} else {
			j.Close()
		}
	}
	windows.ResumeThread(pi.Thread)
	windows.CloseHandle(pi.Thread)
	return nil
}

// commandLine returns the program to start and its command line. A .cmd or
// .bat script (claude as npm installs it) is run by cmd.exe.
func commandLine(path string) (app, cmdLine string) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cmd", ".bat":
		comspec := os.Getenv("ComSpec")
		if comspec == "" {
			comspec = filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
		}
		// /s takes the outer quotes off what follows /c, leaving the
		// quoted path intact.
		return comspec, `cmd.exe /d /s /c ""` + path + `" --strict-mcp-config"`
	}
	return path, windows.EscapeArg(path) + " --strict-mcp-config"
}

// envBlock is env as CreateProcess takes it: NUL-terminated UTF-16 strings,
// with one more NUL at the end.
func envBlock(env []string) []uint16 {
	var b []uint16
	for _, kv := range env {
		if strings.IndexByte(kv, 0) >= 0 {
			continue
		}
		b = append(b, utf16.Encode([]rune(kv))...)
		b = append(b, 0)
	}
	if len(b) == 0 {
		b = append(b, 0)
	}
	return append(b, 0)
}

// stop ends claude and what it started, then closes the pseudo console. The
// reader of the output sees the end once the pseudo console is closed.
func (t *winTerminal) stop() {
	if t.job != nil {
		t.job.Terminate()
	} else if t.process != 0 {
		windows.TerminateProcess(t.process, 1)
	}
	if t.process != 0 {
		windows.WaitForSingleObject(t.process, 5000)
		windows.CloseHandle(t.process)
		t.process = 0
	}
	if t.job != nil {
		t.job.Close()
	}
	// Closing the pseudo console waits, on some versions of Windows, for its
	// output to be read; the reader in watch keeps reading until the end,
	// but do not hang here should it not.
	done := make(chan struct{})
	go func() {
		windows.ClosePseudoConsole(t.console)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
	windows.CloseHandle(t.input)
	t.File.Close()
}
