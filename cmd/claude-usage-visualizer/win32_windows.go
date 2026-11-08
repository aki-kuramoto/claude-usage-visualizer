//go:build windows

package main

// The parts of the Win32 API the Windows window uses, called through
// golang.org/x/sys/windows, which has the DLL loading but few of the GUI
// functions themselves.

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	comctl32 = windows.NewLazySystemDLL("comctl32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW              = user32.NewProc("RegisterClassExW")
	procCreateWindowExW               = user32.NewProc("CreateWindowExW")
	procDefWindowProcW                = user32.NewProc("DefWindowProcW")
	procDestroyWindow                 = user32.NewProc("DestroyWindow")
	procGetMessageW                   = user32.NewProc("GetMessageW")
	procTranslateMessage              = user32.NewProc("TranslateMessage")
	procDispatchMessageW              = user32.NewProc("DispatchMessageW")
	procPostMessageW                  = user32.NewProc("PostMessageW")
	procSendMessageW                  = user32.NewProc("SendMessageW")
	procPostQuitMessage               = user32.NewProc("PostQuitMessage")
	procShowWindow                    = user32.NewProc("ShowWindow")
	procIsWindowVisible               = user32.NewProc("IsWindowVisible")
	procIsIconic                      = user32.NewProc("IsIconic")
	procInvalidateRect                = user32.NewProc("InvalidateRect")
	procBeginPaint                    = user32.NewProc("BeginPaint")
	procEndPaint                      = user32.NewProc("EndPaint")
	procGetClientRect                 = user32.NewProc("GetClientRect")
	procGetWindowRect                 = user32.NewProc("GetWindowRect")
	procSetWindowPos                  = user32.NewProc("SetWindowPos")
	procLoadCursorW                   = user32.NewProc("LoadCursorW")
	procSetCapture                    = user32.NewProc("SetCapture")
	procReleaseCapture                = user32.NewProc("ReleaseCapture")
	procCreatePopupMenu               = user32.NewProc("CreatePopupMenu")
	procAppendMenuW                   = user32.NewProc("AppendMenuW")
	procTrackPopupMenuEx              = user32.NewProc("TrackPopupMenuEx")
	procDestroyMenu                   = user32.NewProc("DestroyMenu")
	procSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	procGetCursorPos                  = user32.NewProc("GetCursorPos")
	procClientToScreen                = user32.NewProc("ClientToScreen")
	procGetDpiForWindow               = user32.NewProc("GetDpiForWindow")
	procGetDpiForSystem               = user32.NewProc("GetDpiForSystem")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetSystemMetricsForDpi        = user32.NewProc("GetSystemMetricsForDpi")
	procMonitorFromPoint              = user32.NewProc("MonitorFromPoint")
	procMonitorFromWindow             = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW               = user32.NewProc("GetMonitorInfoW")
	procCreateAcceleratorTableW       = user32.NewProc("CreateAcceleratorTableW")
	procTranslateAcceleratorW         = user32.NewProc("TranslateAcceleratorW")
	procIsDialogMessageW              = user32.NewProc("IsDialogMessageW")
	procSetWindowTextW                = user32.NewProc("SetWindowTextW")
	procGetWindowTextW                = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW          = user32.NewProc("GetWindowTextLengthW")
	procSetFocus                      = user32.NewProc("SetFocus")
	procSetTimer                      = user32.NewProc("SetTimer")
	procRegisterWindowMessageW        = user32.NewProc("RegisterWindowMessageW")
	procCreateIconIndirect            = user32.NewProc("CreateIconIndirect")
	procDestroyIcon                   = user32.NewProc("DestroyIcon")
	procGetDC                         = user32.NewProc("GetDC")
	procReleaseDC                     = user32.NewProc("ReleaseDC")
	procFillRect                      = user32.NewProc("FillRect")
	procDrawTextW                     = user32.NewProc("DrawTextW")
	procGetSysColor                   = user32.NewProc("GetSysColor")
	procGetSysColorBrush              = user32.NewProc("GetSysColorBrush")
	procGetKeyState                   = user32.NewProc("GetKeyState")
	procMessageBoxW                   = user32.NewProc("MessageBoxW")
	procEnableWindow                  = user32.NewProc("EnableWindow")
	procGetForegroundWindow           = user32.NewProc("GetForegroundWindow")

	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procCreateDIBSection       = gdi32.NewProc("CreateDIBSection")
	procCreateBitmap           = gdi32.NewProc("CreateBitmap")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	procCreatePen              = gdi32.NewProc("CreatePen")
	procCreateFontW            = gdi32.NewProc("CreateFontW")
	procSetTextColor           = gdi32.NewProc("SetTextColor")
	procSetBkMode              = gdi32.NewProc("SetBkMode")
	procSetBkColor             = gdi32.NewProc("SetBkColor")
	procGetTextExtentPoint32W  = gdi32.NewProc("GetTextExtentPoint32W")
	procGetTextMetricsW        = gdi32.NewProc("GetTextMetricsW")
	procPolygon                = gdi32.NewProc("Polygon")
	procEllipse                = gdi32.NewProc("Ellipse")
	procMoveToEx               = gdi32.NewProc("MoveToEx")
	procLineTo                 = gdi32.NewProc("LineTo")
	procGetStockObject         = gdi32.NewProc("GetStockObject")
	procGdiFlush               = gdi32.NewProc("GdiFlush")

	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")

	procInitCommonControlsEx = comctl32.NewProc("InitCommonControlsEx")
	procTaskDialogIndirect   = comctl32.NewProc("TaskDialogIndirect")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

// Window messages and the constants that go with them.
const (
	WM_CREATE           = 0x0001
	WM_DESTROY          = 0x0002
	WM_SIZE             = 0x0005
	WM_ACTIVATE         = 0x0006
	WM_SETFOCUS         = 0x0007
	WM_PAINT            = 0x000F
	WM_CLOSE            = 0x0010
	WM_ERASEBKGND       = 0x0014
	WM_SETCURSOR        = 0x0020
	WM_GETMINMAXINFO    = 0x0024
	WM_SETFONT          = 0x0030
	WM_GETFONT          = 0x0031
	WM_NCHITTEST        = 0x0084
	WM_NCLBUTTONDBLCLK  = 0x00A3
	WM_KEYDOWN          = 0x0100
	WM_SYSKEYDOWN       = 0x0104
	WM_COMMAND          = 0x0111
	WM_SYSCOMMAND       = 0x0112
	WM_TIMER            = 0x0113
	WM_INITMENUPOPUP    = 0x0117
	WM_CTLCOLOREDIT     = 0x0133
	WM_CTLCOLORBTN      = 0x0135
	WM_CTLCOLORSTATIC   = 0x0138
	WM_MOUSEMOVE        = 0x0200
	WM_LBUTTONDOWN      = 0x0201
	WM_LBUTTONUP        = 0x0202
	WM_LBUTTONDBLCLK    = 0x0203
	WM_RBUTTONUP        = 0x0205
	WM_CAPTURECHANGED   = 0x0215
	WM_POWERBROADCAST   = 0x0218
	WM_DPICHANGED       = 0x02E0
	WM_PRINT            = 0x0317
	WM_PRINTCLIENT      = 0x0318
	WM_NULL             = 0x0000
	WM_CONTEXTMENU      = 0x007B
	WM_SETICON          = 0x0080
	WM_APP              = 0x8000
	WM_USER             = 0x0400
	WM_NCLBUTTONDOWN    = 0x00A1
	WM_EXITSIZEMOVE     = 0x0232
	WM_DISPLAYCHANGE    = 0x007E
	WM_SETTINGCHANGE    = 0x001A
	WM_ACTIVATEAPP      = 0x001C
	WM_MOUSEACTIVATE    = 0x0021
	WM_QUERYENDSESSION  = 0x0011
	WM_ENDSESSION       = 0x0016
	WM_GETDLGCODE       = 0x0087
	WM_NCDESTROY        = 0x0082
	WM_SHOWWINDOW       = 0x0018
	WM_WINDOWPOSCHANGED = 0x0047

	WA_INACTIVE = 0

	HTCLIENT  = 1
	HTCAPTION = 2

	PBT_APMSUSPEND         = 0x0004
	PBT_APMRESUMEAUTOMATIC = 0x0012

	SC_CLOSE    = 0xF060
	SC_MINIMIZE = 0xF020
	SC_KEYMENU  = 0xF100

	ICON_SMALL = 0
	ICON_BIG   = 1

	PRF_CHECKVISIBLE = 0x1
	PRF_NONCLIENT    = 0x2
	PRF_CLIENT       = 0x4
	PRF_ERASEBKGND   = 0x8
	PRF_CHILDREN     = 0x10
)

// Window styles.
const (
	WS_OVERLAPPED       = 0x00000000
	WS_POPUP            = 0x80000000
	WS_CHILD            = 0x40000000
	WS_VISIBLE          = 0x10000000
	WS_CLIPCHILDREN     = 0x02000000
	WS_CLIPSIBLINGS     = 0x04000000
	WS_CAPTION          = 0x00C00000
	WS_BORDER           = 0x00800000
	WS_SYSMENU          = 0x00080000
	WS_MINIMIZEBOX      = 0x00020000
	WS_TABSTOP          = 0x00010000
	WS_GROUP            = 0x00020000
	WS_VSCROLL          = 0x00200000
	WS_EX_TOPMOST       = 0x00000008
	WS_EX_APPWINDOW     = 0x00040000
	WS_EX_CONTROLPARENT = 0x00010000
	WS_EX_CLIENTEDGE    = 0x00000200
	WS_EX_DLGMODALFRAME = 0x00000001

	CS_DBLCLKS    = 0x0008
	CS_DROPSHADOW = 0x00020000
	CS_HREDRAW    = 0x0002
	CS_VREDRAW    = 0x0001

	SW_HIDE       = 0
	SW_SHOWNORMAL = 1
	SW_SHOW       = 5
	SW_MINIMIZE   = 6
	SW_RESTORE    = 9
	SW_SHOWNA     = 8

	SWP_NOSIZE       = 0x0001
	SWP_NOMOVE       = 0x0002
	SWP_NOZORDER     = 0x0004
	SWP_NOACTIVATE   = 0x0010
	SWP_FRAMECHANGED = 0x0020
	SWP_SHOWWINDOW   = 0x0040

	HWND_TOPMOST   = ^uintptr(0) // -1
	HWND_NOTOPMOST = ^uintptr(1) // -2
	HWND_MESSAGE   = ^uintptr(2) // -3
	HWND_BROADCAST = 0xFFFF

	IDC_ARROW = 32512

	COLOR_WINDOW     = 5
	COLOR_BTNFACE    = 15
	COLOR_GRAYTEXT   = 17
	COLOR_WINDOWTEXT = 8

	MONITOR_DEFAULTTONULL    = 0
	MONITOR_DEFAULTTONEAREST = 2

	SM_CXSMICON = 49
	SM_CYSMICON = 50
	SM_CXICON   = 11
	SM_CYICON   = 12
)

// Controls.
const (
	BS_PUSHBUTTON    = 0x0
	BS_DEFPUSHBUTTON = 0x1
	BS_AUTOCHECKBOX  = 0x3
	BS_MULTILINE     = 0x2000
	BM_GETCHECK      = 0x00F0
	BM_SETCHECK      = 0x00F1
	BM_SETSTYLE      = 0x00F4
	BST_CHECKED      = 1
	BN_CLICKED       = 0

	ES_LEFT         = 0x0
	ES_MULTILINE    = 0x4
	ES_AUTOHSCROLL  = 0x80
	ES_READONLY     = 0x800
	ES_NUMBER       = 0x2000
	EM_SETCUEBANNER = 0x1501
	EM_SETSEL       = 0x00B1
	EM_LIMITTEXT    = 0x00C5

	SS_LEFT     = 0x0
	SS_NOPREFIX = 0x80

	CBS_DROPDOWNLIST = 0x3
	CB_ADDSTRING     = 0x0143
	CB_GETCURSEL     = 0x0147
	CB_SETCURSEL     = 0x014E
	CB_RESETCONTENT  = 0x014B

	UDS_SETBUDDYINT = 0x2
	UDS_ALIGNRIGHT  = 0x4
	UDS_ARROWKEYS   = 0x20
	UDS_NOTHOUSANDS = 0x80
	UDM_SETRANGE32  = WM_USER + 111
	UDM_SETPOS32    = WM_USER + 113
	UDM_GETPOS32    = WM_USER + 114

	IDOK     = 1
	IDCANCEL = 2

	ICC_STANDARD_CLASSES = 0x4000
	ICC_UPDOWN_CLASS     = 0x10
	ICC_WIN95_CLASSES    = 0xFF
)

// Menus.
const (
	MF_STRING    = 0x0
	MF_SEPARATOR = 0x800
	MF_CHECKED   = 0x8
	MF_GRAYED    = 0x1
	MF_DEFAULT   = 0x1000

	TPM_LEFTALIGN   = 0x0
	TPM_RIGHTBUTTON = 0x2
	TPM_RETURNCMD   = 0x100
	TPM_TOPALIGN    = 0x0
	TPM_BOTTOMALIGN = 0x20
	TPM_NONOTIFY    = 0x80
)

// Keyboard accelerators.
const (
	FVIRTKEY     = 0x01
	FSHIFT       = 0x04
	FCONTROL     = 0x08
	VK_OEM_COMMA = 0xBC
	VK_F4        = 0x73
	VK_MENU      = 0x12
	VK_F10       = 0x79
)

// GDI.
const (
	TRANSPARENT    = 1
	OPAQUE         = 2
	SRCCOPY        = 0x00CC0020
	PS_SOLID       = 0
	NULL_PEN       = 8
	NULL_BRUSH     = 5
	BI_RGB         = 0
	DIB_RGB_COLORS = 0

	FW_NORMAL           = 400
	FW_BOLD             = 700
	DEFAULT_CHARSET     = 1
	OUT_DEFAULT_PRECIS  = 0
	CLIP_DEFAULT_PRECIS = 0
	CLEARTYPE_QUALITY   = 5
	DEFAULT_PITCH       = 0

	DT_LEFT         = 0x0
	DT_CENTER       = 0x1
	DT_RIGHT        = 0x2
	DT_VCENTER      = 0x4
	DT_SINGLELINE   = 0x20
	DT_WORDBREAK    = 0x10
	DT_CALCRECT     = 0x400
	DT_NOPREFIX     = 0x800
	DT_END_ELLIPSIS = 0x8000
	DT_EDITCONTROL  = 0x2000
)

// The notification area.
const (
	NIM_ADD        = 0
	NIM_MODIFY     = 1
	NIM_DELETE     = 2
	NIM_SETVERSION = 4

	NIF_MESSAGE = 0x1
	NIF_ICON    = 0x2
	NIF_TIP     = 0x4
	NIF_INFO    = 0x10
	NIF_SHOWTIP = 0x80

	NIIF_USER       = 0x4
	NIIF_LARGE_ICON = 0x20
	NIIF_WARNING    = 0x2

	NOTIFYICON_VERSION_4 = 4

	NIN_SELECT           = WM_USER + 0
	NIN_KEYSELECT        = NIN_SELECT | 1
	NIN_BALLOONUSERCLICK = WM_USER + 5
)

// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 is (DPI_AWARENESS_CONTEXT)-4.
const dpiAwarenessPerMonitorV2 = ^uintptr(3)

type WNDCLASSEXW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type POINT struct{ X, Y int32 }

type RECT struct{ Left, Top, Right, Bottom int32 }

type MSG struct {
	Hwnd    windows.HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
	private uint32
}

type PAINTSTRUCT struct {
	Hdc         windows.Handle
	Erase       int32
	Paint       RECT
	Restore     int32
	IncUpdate   int32
	RGBReserved [32]byte
}

type MONITORINFO struct {
	Size    uint32
	Monitor RECT
	Work    RECT
	Flags   uint32
}

type ACCEL struct {
	Virt uint8
	Key  uint16
	Cmd  uint16
}

type SIZE struct{ CX, CY int32 }

type TEXTMETRICW struct {
	Height, Ascent, Descent, InternalLeading, ExternalLeading int32
	AveCharWidth, MaxCharWidth, Weight, Overhang              int32
	DigitizedAspectX, DigitizedAspectY                        int32
	FirstChar, LastChar, DefaultChar, BreakChar               uint16
	Italic, Underlined, StruckOut, PitchAndFamily, CharSet    uint8
}

type ICONINFO struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	Mask     windows.Handle
	Color    windows.Handle
}

type BITMAPINFOHEADER struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type NOTIFYICONDATAW struct {
	Size            uint32
	Wnd             windows.HWND
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            windows.Handle
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUIDItem        windows.GUID
	BalloonIcon     windows.Handle
}

type INITCOMMONCONTROLSEX struct {
	Size uint32
	ICC  uint32
}

type MINMAXINFO struct {
	Reserved     POINT
	MaxSize      POINT
	MaxPosition  POINT
	MinTrackSize POINT
	MaxTrackSize POINT
}

// call runs a Win32 procedure and returns its first result, ignoring the
// error that LazyProc.Call always returns.
//
// A pointer passed to it must be converted right in the argument list, as
// uintptr(unsafe.Pointer(&x)): go:uintptrescapes then moves x to the heap
// and keeps it alive until the call returns, as LazyProc.Call itself does.
// Without that, x could stay on the goroutine's stack, which moves when the
// stack grows, and Windows would read whatever is left at the old place;
// converted earlier, in a helper, x could be freed during the call.
//
//go:uintptrescapes
func call(p *windows.LazyProc, args ...uintptr) uintptr {
	r, _, _ := p.Call(args...)
	return r
}

func utf16Ptr(s string) *uint16 {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		p, _ = windows.UTF16PtrFromString("")
	}
	return p
}

func boolArg(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

func loword(v uintptr) uint16 { return uint16(v) }
func hiword(v uintptr) uint16 { return uint16(v >> 16) }

// pointFromLParam reads the signed coordinates packed into lParam.
func pointFromLParam(l uintptr) (int32, int32) {
	return int32(int16(loword(l))), int32(int16(hiword(l)))
}

func rgb(r, g, b uint8) uint32 { return uint32(r) | uint32(g)<<8 | uint32(b)<<16 }

// sendMessage and postMessage take pointers the way call does.
//
//go:uintptrescapes
func sendMessage(hwnd windows.HWND, msg uint32, w, l uintptr) uintptr {
	return call(procSendMessageW, uintptr(hwnd), uintptr(msg), w, l)
}

//go:uintptrescapes
func postMessage(hwnd windows.HWND, msg uint32, w, l uintptr) {
	call(procPostMessageW, uintptr(hwnd), uintptr(msg), w, l)
}

func setWindowText(hwnd windows.HWND, s string) {
	call(procSetWindowTextW, uintptr(hwnd), uintptr(unsafe.Pointer(utf16Ptr(s))))
}

func getWindowText(hwnd windows.HWND) string {
	n := call(procGetWindowTextLengthW, uintptr(hwnd))
	buf := make([]uint16, n+1)
	call(procGetWindowTextW, uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), n+1)
	return windows.UTF16ToString(buf)
}

func copyUTF16(dst []uint16, s string) {
	u, _ := windows.UTF16FromString(s)
	if len(u) > len(dst) {
		u = u[:len(dst)]
		u[len(u)-1] = 0
	}
	copy(dst, u)
}

func moduleHandle() windows.Handle {
	return windows.Handle(call(procGetModuleHandleW, 0))
}

// newCallback wraps a window procedure for Windows to call.
func newCallback(f func(hwnd windows.HWND, msg uint32, w, l uintptr) uintptr) uintptr {
	return syscall.NewCallback(f)
}

// rectAt reads the RECT that lParam points to, as WM_DPICHANGED passes it.
func rectAt(l uintptr) RECT {
	return *(*RECT)(unsafe.Add(unsafe.Pointer(nil), l))
}
