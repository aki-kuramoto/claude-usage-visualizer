package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
)

// stderrLogMax is the size past which stderr.log is started afresh, so that
// it never grows without end.
const stderrLogMax = 1 << 20

// keepStderr sends standard error, and the report Go writes when the
// program crashes (an unhandled panic or a fatal runtime error), to
// stderr.log in dir.
//
// The window application has nowhere else for them: on Windows it is built
// without a console, and on macOS it is started by Finder, so whatever it
// writes to standard error is lost, and a crash leaves no trace of why. Each
// start adds one line to the file, so a crash report can be told apart
// from the start it belongs to.
func keepStderr(dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	path := filepath.Join(dir, "stderr.log")
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if fi, err := os.Stat(path); err == nil && fi.Size() > stderrLogMax {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return
	}
	exe, _ := os.Executable()
	fmt.Fprintf(f, "--- started %s, pid %d, %s\n", time.Now().Format(time.RFC3339), os.Getpid(), exe)
	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		fmt.Fprintln(f, "the crash report cannot be kept here:", err)
	}
	// SetCrashOutput keeps its own copy of the file; this one serves
	// whatever the program itself writes to standard error.
	os.Stderr = f
}
