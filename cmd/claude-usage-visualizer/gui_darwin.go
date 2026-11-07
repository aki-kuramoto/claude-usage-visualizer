//go:build darwin

package main

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework UserNotifications
#include <stdlib.h>
#include "gui_darwin.h"
*/
import "C"

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/app"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/oplog"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/paths"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/settings"
)

// AppKit must run on the main thread, and the main goroutine starts there;
// lock it so that it stays.
func init() {
	runtime.LockOSThread()
}

// The window calls back into Go through the exported functions below, which
// reach the controller through these.
var (
	controller *app.Controller
	stopRun    context.CancelFunc
	runDone    chan struct{}
)

func runGUI(p paths.Paths, log *oplog.Log, s settings.Settings) error {
	controller = newController(p, log, s, func(st app.State) {
		data, err := json.Marshal(st)
		if err != nil {
			return
		}
		cs := C.CString(string(data))
		defer C.free(unsafe.Pointer(cs))
		C.cuvUpdate(cs)
	})
	langs := C.cuvPreferredLanguages()
	controller.SetPreferredLanguages(strings.Split(C.GoString(langs), "\n"))
	C.free(unsafe.Pointer(langs))

	var ctx context.Context
	ctx, stopRun = context.WithCancel(context.Background())
	runDone = make(chan struct{})
	go func() {
		controller.Run(ctx)
		close(runDone)
	}()
	C.cuvRun() // never returns; the process ends from the AppKit side
	return nil
}

//export goFetchNow
func goFetchNow() {
	controller.FetchNow("manual")
}

// goSaveSettings takes the settings as JSON (the same names as in State) and
// returns NULL on success, or an error message that the caller frees.
//
//export goSaveSettings
func goSaveSettings(settingsJSON *C.char) *C.char {
	s := controller.State().Settings
	if err := json.Unmarshal([]byte(C.GoString(settingsJSON)), &s); err != nil {
		return C.CString(err.Error())
	}
	if err := controller.SaveSettings(s); err != nil {
		return C.CString(err.Error())
	}
	return nil
}

// goDaysElapsed tells how much of the period ending at resetUnix (seconds
// since 1970) has passed, counting days by their weight in days.setay, or
// -1 when there is nothing to show.
//
//export goDaysElapsed
func goDaysElapsed(resetUnix, windowSeconds C.double) C.double {
	reset := time.Unix(0, int64(float64(resetUnix)*1e9))
	window := time.Duration(float64(windowSeconds) * float64(time.Second))
	frac, ok := controller.DaysElapsed(reset, window, time.Now())
	if !ok {
		return -1
	}
	return C.double(frac)
}

// goDaysAhead is like goDaysElapsed, judged by the days still ahead: it
// matches the plain share of time when every day ahead is of the heaviest
// kind. It returns -1 when there is nothing to show.
//
//export goDaysAhead
func goDaysAhead(resetUnix, windowSeconds C.double) C.double {
	reset := time.Unix(0, int64(float64(resetUnix)*1e9))
	window := time.Duration(float64(windowSeconds) * float64(time.Second))
	frac, ok := controller.DaysAhead(reset, window, time.Now())
	if !ok {
		return -1
	}
	return C.double(frac)
}

// goDaysError says why days.setay cannot be used, or returns NULL. The caller
// frees the result.
//
//export goDaysError
func goDaysError() *C.char {
	if msg := controller.DaysError(); msg != "" {
		return C.CString(msg)
	}
	return nil
}

// goOpenLogin opens Terminal running "claude auth login", the way to log in
// again. It writes a small script and opens it: opening a .command file runs
// it in Terminal without asking for permission to control Terminal. It
// returns NULL, or an error message that the caller frees.
//
//export goOpenLogin
func goOpenLogin() *C.char {
	script := filepath.Join(controller.WorkDir(), "login.command")
	err := func() error {
		path, err := controller.ClaudePath()
		if err != nil {
			return err
		}
		body := "#!/bin/sh\n" +
			"# Written by claude-usage-visualizer to log Claude Code in again.\n" +
			"exec " + shellQuote(path) + " auth login\n"
		if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
			return err
		}
		if err := os.Chmod(script, 0o755); err != nil {
			return err
		}
		return exec.Command("open", "-a", "Terminal", script).Run()
	}()
	controller.NoteLoginOpened(script, err)
	if err != nil {
		return C.CString(err.Error())
	}
	return nil
}

// goOpenTrust opens Terminal running the interactive claude in the working
// directory, so that the user can answer its "trust this folder?" question
// once; the login expiry check needs that. A message saying why is printed
// first. It returns NULL, or an error message that the caller frees.
//
//export goOpenTrust
func goOpenTrust() *C.char {
	script := filepath.Join(controller.WorkDir(), "trust.command")
	err := func() error {
		path, err := controller.ClaudePath()
		if err != nil {
			return err
		}
		msg := controller.State().Strings["trust_script_message"]
		body := "#!/bin/sh\n" +
			"# Written by claude-usage-visualizer to let the user trust its working folder once.\n" +
			"cd " + shellQuote(controller.WorkDir()) + " || exit 1\n" +
			"printf '%s\\n\\n' " + shellQuote(msg) + "\n" +
			"exec " + shellQuote(path) + " --strict-mcp-config\n"
		if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
			return err
		}
		if err := os.Chmod(script, 0o755); err != nil {
			return err
		}
		return exec.Command("open", "-a", "Terminal", script).Run()
	}()
	controller.NoteTrustOpened(script, err)
	if err != nil {
		return C.CString(err.Error())
	}
	return nil
}

// shellQuote quotes s for /bin/sh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

//export goNoteWindow
func goNoteWindow(what *C.char) {
	controller.NoteWindow(C.GoString(what))
}

//export goNotePower
func goNotePower(what *C.char) {
	controller.NotePower(C.GoString(what))
}

// goWillTerminate stops the controller (and with it claude) before the
// process ends, waiting a little for it to finish.
//
//export goWillTerminate
func goWillTerminate() {
	stopRun()
	select {
	case <-runDone:
	case <-time.After(8 * time.Second):
	}
}
