//go:build windows

package main

// The icon in the notification area, its menu and its notifications, and the
// question asked when the window is closed.

import (
	"runtime"
	"unsafe"
)

const trayID = 1

func (g *gui) trayData() NOTIFYICONDATAW {
	nid := NOTIFYICONDATAW{Wnd: g.hwnd, ID: trayID}
	nid.Size = uint32(unsafe.Sizeof(nid))
	return nid
}

// trayTip is the icon's tooltip: the window's title and, below it, the
// compact view's line.
func (g *gui) trayTip() string {
	return g.T("window_title") + "\n" + compactTitle(g.state)
}

func (g *gui) addTray() {
	if g.tray || g.hidden {
		return
	}
	g.makeTrayIcon()
	nid := g.trayData()
	nid.Flags = NIF_MESSAGE | NIF_ICON | NIF_TIP | NIF_SHOWTIP
	nid.CallbackMessage = wmTray
	nid.Icon = g.trayIcon
	copyUTF16(nid.Tip[:], g.trayTip())
	if call(procShellNotifyIconW, NIM_ADD, uintptr(unsafe.Pointer(&nid))) == 0 {
		return
	}
	nid.Version = NOTIFYICON_VERSION_4
	call(procShellNotifyIconW, NIM_SETVERSION, uintptr(unsafe.Pointer(&nid)))
	g.tray = true
}

func (g *gui) removeTray() {
	if !g.tray {
		return
	}
	nid := g.trayData()
	call(procShellNotifyIconW, NIM_DELETE, uintptr(unsafe.Pointer(&nid)))
	g.tray = false
}

// makeTrayIcon makes the icon at the size the notification area wants, with
// the badge while claude is not logged in.
func (g *gui) makeTrayIcon() {
	n := int(call(procGetSystemMetricsForDpi, SM_CXSMICON, uintptr(g.dpi)))
	if n <= 0 {
		n = 16
	}
	icon := appIcon(n, g.trayBadge)
	if g.trayIcon != 0 {
		call(procDestroyIcon, uintptr(g.trayIcon))
	}
	g.trayIcon = icon
}

// updateTray brings the tooltip up to date.
func (g *gui) updateTray() {
	if !g.tray {
		return
	}
	nid := g.trayData()
	nid.Flags = NIF_TIP | NIF_SHOWTIP
	copyUTF16(nid.Tip[:], g.trayTip())
	call(procShellNotifyIconW, NIM_MODIFY, uintptr(unsafe.Pointer(&nid)))
}

func (g *gui) updateTrayIcon() {
	if !g.tray {
		return
	}
	g.makeTrayIcon()
	nid := g.trayData()
	nid.Flags = NIF_ICON
	nid.Icon = g.trayIcon
	call(procShellNotifyIconW, NIM_MODIFY, uintptr(unsafe.Pointer(&nid)))
}

// notify shows a notification from the tray icon. Windows 10 and later show
// it as a toast.
func (g *gui) notify(title, body string) {
	if !g.tray {
		return
	}
	nid := g.trayData()
	nid.Flags = NIF_INFO
	copyUTF16(nid.InfoTitle[:], title)
	copyUTF16(nid.Info[:], body)
	nid.InfoFlags = NIIF_WARNING
	call(procShellNotifyIconW, NIM_MODIFY, uintptr(unsafe.Pointer(&nid)))
}

// trayEvent handles the tray icon's events: a click (or Enter) shows the
// window, the context menu offers the rest, and clicking the notification
// shows the window.
func (g *gui) trayEvent(event uint16) {
	switch uint32(event) {
	case NIN_SELECT, NIN_KEYSELECT, NIN_BALLOONUSERCLICK:
		g.show()
	case WM_CONTEXTMENU:
		g.trayMenu()
	}
}

func (g *gui) trayMenu() {
	m := call(procCreatePopupMenu)
	defer call(procDestroyMenu, m)
	add := func(flags uintptr, id int, s string) {
		call(procAppendMenuW, m, flags, uintptr(id), ptr(utf16Ptr(s)))
	}
	add(MF_STRING|MF_DEFAULT, cmdShow, g.T("menu_show"))
	add(MF_STRING, cmdFetch, g.T("menu_fetch_now"))
	add(MF_STRING, cmdSettings, g.T("menu_settings"))
	add(MF_SEPARATOR, 0, "")
	add(MF_STRING, cmdQuit, fill(g.T("menu_quit"), "app", appName))

	var pt POINT
	call(procGetCursorPos, uintptr(unsafe.Pointer(&pt)))
	// Without this the menu does not close when clicking elsewhere.
	call(procSetForegroundWindow, uintptr(g.hwnd))
	cmd := call(procTrackPopupMenuEx, m, TPM_RIGHTBUTTON|TPM_BOTTOMALIGN|TPM_RETURNCMD|TPM_NONOTIFY,
		uintptr(pt.X), uintptr(pt.Y), uintptr(g.hwnd), 0)
	postMessage(g.hwnd, WM_NULL, 0, 0)
	if cmd != 0 {
		g.command(int(cmd))
	}
}

// --- the question asked when the window is closed ------------------------------

type closeChoice int

const (
	closeChoiceCancel closeChoice = iota
	closeChoiceQuit
	closeChoiceClose
)

const (
	tdfAllowDialogCancellation  = 0x0008
	tdfPositionRelativeToWindow = 0x1000
	tdWarningIcon               = 0xFFFF // MAKEINTRESOURCE(-1)
	tdInformationIcon           = 0xFFFD // MAKEINTRESOURCE(-3)
	idQuit                      = 1001
	idJustClose                 = 1002
)

// askClose asks whether to quit or only close the window, with the buttons
// the macOS window's sheet has.
func (g *gui) askClose() closeChoice {
	title := utf16Ptr(g.T("close_alert_title"))
	message := utf16Ptr(g.T("close_alert_message") + "\n" + g.T("tray_hint"))
	caption := utf16Ptr(g.T("window_title"))
	quit := utf16Ptr(g.T("close_alert_quit"))
	justClose := utf16Ptr(g.T("close_alert_close"))
	cancel := utf16Ptr(g.T("close_alert_cancel"))
	// A note at the foot: the question can be turned off in the settings.
	footer := utf16Ptr(g.T("close_alert_setting"))

	if procTaskDialogIndirect.Find() != nil {
		// Without version 6 of the common controls (a build without the
		// manifest): Yes, No and Cancel stand for the three.
		r := call(procMessageBoxW, uintptr(g.hwnd),
			ptr(utf16Ptr(g.T("close_alert_title")+"\n\n"+g.T("close_alert_message")+"\n"+g.T("close_alert_setting")+"\n\n"+
				g.T("close_alert_quit")+" / "+g.T("close_alert_close")+" / "+g.T("close_alert_cancel"))),
			ptr(caption), 0x23) // MB_YESNOCANCEL | MB_ICONQUESTION
		switch r {
		case 6: // IDYES
			return closeChoiceQuit
		case 7: // IDNO
			return closeChoiceClose
		}
		return closeChoiceCancel
	}

	// TASKDIALOGCONFIG and TASKDIALOG_BUTTON are packed to one byte, so
	// they are written field by field.
	var buttons taskDialogBytes
	for _, b := range []struct {
		id   int32
		text *uint16
	}{{idQuit, quit}, {idJustClose, justClose}, {IDCANCEL, cancel}} {
		buttons.u32(uint32(b.id))
		buttons.ptr(uintptr(unsafe.Pointer(b.text)))
	}
	var c taskDialogBytes
	c.u32(0) // cbSize, filled in below
	c.ptr(uintptr(g.hwnd))
	c.ptr(0) // hInstance
	c.u32(tdfAllowDialogCancellation | tdfPositionRelativeToWindow)
	c.u32(0) // dwCommonButtons
	c.ptr(uintptr(unsafe.Pointer(caption)))
	c.ptr(tdWarningIcon)
	c.ptr(uintptr(unsafe.Pointer(title)))
	c.ptr(uintptr(unsafe.Pointer(message)))
	c.u32(3) // cButtons
	c.ptr(uintptr(unsafe.Pointer(&buttons.b[0])))
	c.u32(idQuit)                          // nDefaultButton
	c.u32(0)                               // cRadioButtons
	c.ptr(0)                               // pRadioButtons
	c.u32(0)                               // nDefaultRadioButton
	c.ptr(0)                               // pszVerificationText
	c.ptr(0)                               // pszExpandedInformation
	c.ptr(0)                               // pszExpandedControlText
	c.ptr(0)                               // pszCollapsedControlText
	c.ptr(tdInformationIcon)               // hFooterIcon
	c.ptr(uintptr(unsafe.Pointer(footer))) // pszFooter
	c.ptr(0)                               // pfCallback
	c.ptr(0)                               // lpCallbackData
	c.u32(0)                               // cxWidth
	size := uint32(len(c.b))
	c.b[0], c.b[1], c.b[2], c.b[3] = byte(size), byte(size>>8), byte(size>>16), byte(size>>24)

	var pressed int32
	hr := call(procTaskDialogIndirect, uintptr(unsafe.Pointer(&c.b[0])), uintptr(unsafe.Pointer(&pressed)), 0, 0)
	runtime.KeepAlive(buttons)
	runtime.KeepAlive([]*uint16{title, message, caption, quit, justClose, cancel, footer})
	if hr != 0 {
		return closeChoiceCancel
	}
	switch pressed {
	case idQuit:
		return closeChoiceQuit
	case idJustClose:
		return closeChoiceClose
	}
	return closeChoiceCancel
}

// taskDialogBytes builds a structure packed to one byte.
type taskDialogBytes struct{ b []byte }

func (t *taskDialogBytes) u32(v uint32) {
	t.b = append(t.b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

func (t *taskDialogBytes) ptr(v uintptr) {
	for i := 0; i < int(unsafe.Sizeof(v)); i++ {
		t.b = append(t.b, byte(v>>(8*i)))
	}
}
