//go:build windows

package main

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/paths"
)

var (
	procCopyAcceleratorTableW   = user32.NewProc("CopyAcceleratorTableW")
	procDestroyAcceleratorTable = user32.NewProc("DestroyAcceleratorTable")
	procUnregisterClassW        = user32.NewProc("UnregisterClassW")
)

// TestAcceleratorTable makes the window, without showing it, and reads its
// keyboard shortcuts back from Windows. They were once all lost: the table
// was read from where the stack used to be, and CreateAcceleratorTableW
// failed (see TestPointersPassedToWin32, which guards against the cause).
func TestAcceleratorTable(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("CUV_WINDOW_AT", "0,0") // not window.json

	g := &gui{paths: paths.Paths{Base: t.TempDir()}, fonts: &fonts{}, openMenu: -1}
	theGUI = g
	defer func() { theGUI = nil }()
	if err := g.createWindow(); err != nil {
		t.Fatal(err)
	}
	// Last: the class, so that the test can run again in this process.
	defer call(procUnregisterClassW, uintptr(unsafe.Pointer(utf16Ptr(mainClass))), uintptr(moduleHandle()))
	defer call(procDestroyWindow, uintptr(g.hwnd))
	if g.accel == 0 {
		t.Fatal("no accelerator table")
	}
	defer call(procDestroyAcceleratorTable, g.accel)

	n := call(procCopyAcceleratorTableW, g.accel, 0, 0)
	got := make([]ACCEL, n)
	if n > 0 {
		call(procCopyAcceleratorTableW, g.accel, uintptr(unsafe.Pointer(&got[0])), n)
	}
	// Windows may report flags of its own (FNOINVERT), so only the ones set
	// here are compared.
	const mods = FVIRTKEY | FCONTROL | FSHIFT
	want := map[ACCEL]bool{
		{FVIRTKEY | FCONTROL, 'R', cmdFetch}:             true,
		{FVIRTKEY | FCONTROL, 'C', cmdCopy}:              true,
		{FVIRTKEY | FCONTROL | FSHIFT, 'C', cmdCompact}:  true,
		{FVIRTKEY | FCONTROL, VK_OEM_COMMA, cmdSettings}: true,
		{FVIRTKEY | FCONTROL, 'W', cmdClose}:             true,
	}
	if len(got) != len(want) {
		t.Errorf("%d shortcuts, want %d: %+v", len(got), len(want), got)
	}
	for _, a := range got {
		a.Virt &= mods
		if !want[a] {
			t.Errorf("unexpected shortcut %+v", a)
		}
		delete(want, a)
	}
	for a := range want {
		t.Errorf("missing shortcut %+v", a)
	}
}
