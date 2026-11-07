//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// attachConsole makes -headless print to the console it was started from.
// The installed executable is a GUI program (no console window of its own
// opens when it starts), which Windows starts without standard handles; when
// they are redirected (2>file), they are kept as they are.
func attachConsole() {
	if h, err := windows.GetStdHandle(windows.STD_ERROR_HANDLE); err == nil && h != 0 && h != windows.InvalidHandle {
		return
	}
	const attachParentProcess = ^uint32(0) // ATTACH_PARENT_PROCESS
	if r, _, _ := kernel32.NewProc("AttachConsole").Call(uintptr(attachParentProcess)); r == 0 {
		return
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stderr = f
		os.Stdout = f
	}
}
