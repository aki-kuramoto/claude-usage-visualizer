//go:build windows

package main

// The Windows window, written with the Win32 API.
//
// Go owns all the logic, as on macOS: the controller hands over its state
// and this file draws it (see draw_windows.go) and passes the user's actions
// back. Every piece of text comes from the state's "strings" table. All the
// windows belong to one thread, locked to the goroutine that runs runGUI.
//
// Closing the window leaves the application running and recording in the
// notification area (the "tray"), whose icon brings the window back and
// whose menu quits. Whether closing asks first follows the close_action
// setting, as on macOS.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/app"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/oplog"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/paths"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/settings"
)

const (
	appName = "claude-usage-visualizer"

	wmState = WM_APP + 1 // a new state is waiting
	wmTray  = WM_APP + 2 // something happened to the tray icon

	timerRedraw = 1

	cmdFetch    = 100
	cmdCompact  = 101
	cmdSettings = 102
	cmdClose    = 103
	cmdShow     = 104
	cmdQuit     = 105
	cmdButton   = 200 // + the index of a button in the contents
)

// The menus of the menu bar inside the window.
const (
	menuUsage = iota
	menuView
	menuSettings
)

const (
	boxNone = iota
	boxClose
	boxShade
)

// gui is the application's windows. Its fields belong to the window thread,
// except pending.
type gui struct {
	ctrl    *app.Controller
	paths   paths.Paths
	stopRun context.CancelFunc
	runDone chan struct{}

	pendingMu sync.Mutex
	pending   *app.State

	hwnd   windows.HWND
	state  app.State
	active bool
	dpi    uint32
	fonts  *fonts
	lay    layout
	accel  uintptr

	compact, compactSettled bool
	lang                    string

	pressed  int // the box held down, while it is under the mouse
	tracking int // the box the mouse went down on
	openMenu int // the menu bar item whose menu is open, or -1

	buttons    []windows.HWND
	buttonKeys []string
	buttonActs []action
	buttonFont windows.Handle

	contentBrush uintptr

	tray              bool
	trayIcon          windows.Handle
	trayBadge         bool
	taskbarCreated    uint32
	showMessage       uint32 // sent by a second copy started for the same directory
	notifiedLoggedOut bool
	windowIconSmall   windows.Handle
	windowIconBig     windows.Handle

	settings *settingsWindow

	hidden       bool   // CUV_HIDDEN: never show a window or a tray icon
	snapshotDir  string // CUV_SNAPSHOT_DIR: write PNGs of the drawing
	snapshotLook string // the look a snapshot is being drawn in
	quitting     bool
}

var theGUI *gui

func runGUI(p paths.Paths, log *oplog.Log, s settings.Settings) error {
	// Windows belong to the thread that made them, and their messages must
	// be read on it.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// The manifest asks for this too; this covers a build without it.
	call(procSetProcessDpiAwarenessContext, dpiAwarenessPerMonitorV2)
	icc := INITCOMMONCONTROLSEX{ICC: ICC_STANDARD_CLASSES | ICC_UPDOWN_CLASS | ICC_WIN95_CLASSES}
	icc.Size = uint32(unsafe.Sizeof(icc))
	call(procInitCommonControlsEx, uintptr(unsafe.Pointer(&icc)))

	g := &gui{
		paths:       p,
		fonts:       &fonts{},
		openMenu:    -1,
		hidden:      os.Getenv("CUV_HIDDEN") != "",
		snapshotDir: os.Getenv("CUV_SNAPSHOT_DIR"),
	}
	theGUI = g
	// One copy per directory of settings and records: a second start
	// shows the window of the first and ends.
	instance := instanceName(p.Base)
	g.showMessage = uint32(call(procRegisterWindowMessageW, ptr(utf16Ptr(instance+"-show"))))
	mutex, err := windows.CreateMutex(nil, false, utf16Ptr(`Local\`+instance))
	if err == windows.ERROR_ALREADY_EXISTS {
		postMessage(windows.HWND(HWND_BROADCAST), g.showMessage, 0, 0)
		log.Add(oplog.KindWindow, "", map[string]any{"what": "already running; showed the running copy"})
		return nil
	}
	defer windows.CloseHandle(mutex)

	g.ctrl = newController(p, log, s, g.publish)
	if langs, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME); err == nil {
		g.ctrl.SetPreferredLanguages(langs)
	}
	g.state = g.ctrl.State()

	if err := g.createWindow(); err != nil {
		return err
	}
	g.render()
	if !g.hidden {
		g.addTray()
		g.show()
	}

	var ctx context.Context
	ctx, g.stopRun = context.WithCancel(context.Background())
	g.runDone = make(chan struct{})
	go func() {
		g.ctrl.Run(ctx)
		close(g.runDone)
	}()

	var msg MSG
	for {
		r := call(procGetMessageW, uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 || int32(r) == -1 {
			break
		}
		if g.settings != nil && g.settings.hwnd != 0 &&
			call(procIsDialogMessageW, uintptr(g.settings.hwnd), uintptr(unsafe.Pointer(&msg))) != 0 {
			continue
		}
		if msg.Hwnd == g.hwnd || isChildOf(msg.Hwnd, g.hwnd) {
			if call(procTranslateAcceleratorW, uintptr(g.hwnd), g.accel, uintptr(unsafe.Pointer(&msg))) != 0 {
				continue
			}
		}
		call(procTranslateMessage, uintptr(unsafe.Pointer(&msg)))
		call(procDispatchMessageW, uintptr(unsafe.Pointer(&msg)))
	}
	return nil
}

var procGetParent = user32.NewProc("GetParent")

// instanceName names this copy by its directory, so that copies for other
// directories (as in testing) can run side by side.
func instanceName(base string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(base))))
	return appName + "-" + hex.EncodeToString(sum[:8])
}

func isChildOf(h, parent windows.HWND) bool {
	for h != 0 {
		h = windows.HWND(call(procGetParent, uintptr(h)))
		if h == parent {
			return true
		}
	}
	return false
}

// publish is called by the controller, on its own goroutine, with each new
// state. The window thread picks it up.
func (g *gui) publish(st app.State) {
	g.pendingMu.Lock()
	g.pending = &st
	g.pendingMu.Unlock()
	if g.hwnd != 0 {
		postMessage(g.hwnd, wmState, 0, 0)
	}
}

// --- the window ---------------------------------------------------------------

const mainClass = "ClaudeUsageVisualizer"

func (g *gui) createWindow() error {
	inst := moduleHandle()
	wc := WNDCLASSEXW{
		Style:     CS_DBLCLKS | CS_DROPSHADOW,
		WndProc:   newCallback(mainProc),
		Instance:  inst,
		Cursor:    windows.Handle(call(procLoadCursorW, 0, IDC_ARROW)),
		ClassName: utf16Ptr(mainClass),
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if call(procRegisterClassExW, uintptr(unsafe.Pointer(&wc))) == 0 {
		return fmt.Errorf("registering the window class failed")
	}
	g.taskbarCreated = uint32(call(procRegisterWindowMessageW, ptr(utf16Ptr("TaskbarCreated"))))

	// Borderless: the title bar is drawn by the application, not by
	// Windows. WS_SYSMENU and WS_MINIMIZEBOX keep the taskbar button's
	// menu and minimizing from it.
	style := uintptr(WS_POPUP | WS_CLIPCHILDREN | WS_SYSMENU | WS_MINIMIZEBOX)
	hwnd := call(procCreateWindowExW, WS_EX_APPWINDOW, ptr(utf16Ptr(mainClass)), ptr(utf16Ptr(appName)),
		style, 0, 0, 380, 200, 0, 0, uintptr(inst), 0)
	if hwnd == 0 {
		return fmt.Errorf("creating the window failed")
	}
	g.hwnd = windows.HWND(hwnd)
	g.dpi = uint32(call(procGetDpiForWindow, hwnd))
	if g.dpi == 0 {
		g.dpi = 96
	}
	g.setWindowIcons()
	g.placeInitially()

	accels := []ACCEL{
		{FVIRTKEY | FCONTROL, 'R', cmdFetch},
		{FVIRTKEY | FCONTROL | FSHIFT, 'C', cmdCompact},
		{FVIRTKEY | FCONTROL, VK_OEM_COMMA, cmdSettings},
		{FVIRTKEY | FCONTROL, 'W', cmdClose},
	}
	g.accel = call(procCreateAcceleratorTableW, uintptr(unsafe.Pointer(&accels[0])), uintptr(len(accels)))
	call(procSetTimer, hwnd, timerRedraw, 30*1000, 0)
	return nil
}

func (g *gui) scale() scale { return scale(float64(g.dpi) / 96) }

func (g *gui) setWindowIcons() {
	s := g.scale()
	small := appIcon(int(s.px(16)), false)
	big := appIcon(int(s.px(32)), false)
	sendMessage(g.hwnd, WM_SETICON, ICON_SMALL, uintptr(small))
	sendMessage(g.hwnd, WM_SETICON, ICON_BIG, uintptr(big))
	if g.windowIconSmall != 0 {
		call(procDestroyIcon, uintptr(g.windowIconSmall))
		call(procDestroyIcon, uintptr(g.windowIconBig))
	}
	g.windowIconSmall, g.windowIconBig = small, big
}

func mainProc(hwnd windows.HWND, msg uint32, w, l uintptr) uintptr {
	g := theGUI
	if g == nil || (g.hwnd != 0 && hwnd != g.hwnd) {
		return call(procDefWindowProcW, uintptr(hwnd), uintptr(msg), w, l)
	}
	if g.hwnd == 0 {
		g.hwnd = hwnd // messages sent while CreateWindowEx runs
	}
	switch msg {
	case wmState:
		g.pendingMu.Lock()
		st := g.pending
		g.pending = nil
		g.pendingMu.Unlock()
		if st != nil {
			g.state = *st
			g.render()
		}
		return 0
	case WM_PAINT:
		var ps PAINTSTRUCT
		dc := windows.Handle(call(procBeginPaint, uintptr(hwnd), uintptr(unsafe.Pointer(&ps))))
		g.paintBuffered(dc, g.active)
		call(procEndPaint, uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
		return 0
	case WM_PRINTCLIENT:
		g.paint(windows.Handle(w), g.forcedLook())
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_ACTIVATE:
		g.active = loword(w) != WA_INACTIVE
		g.invalidate()
		return 0
	case WM_NCHITTEST:
		x, y := pointFromLParam(l)
		pt := POINT{x, y}
		call(procScreenToClient, uintptr(hwnd), uintptr(unsafe.Pointer(&pt)))
		if g.inTitleBar(pt) && g.boxAt(pt) == boxNone {
			return HTCAPTION
		}
		return HTCLIENT
	case WM_NCLBUTTONDBLCLK:
		if w == HTCAPTION {
			g.toggleCompact()
			return 0
		}
	case WM_LBUTTONDOWN, WM_LBUTTONDBLCLK:
		x, y := pointFromLParam(l)
		g.mouseDown(POINT{x, y})
		return 0
	case WM_MOUSEMOVE:
		if g.tracking != boxNone {
			x, y := pointFromLParam(l)
			now := boxNone
			if g.boxAt(POINT{x, y}) == g.tracking {
				now = g.tracking
			}
			if now != g.pressed {
				g.pressed = now
				g.invalidate()
			}
		}
		return 0
	case WM_LBUTTONUP:
		if g.tracking != boxNone {
			x, y := pointFromLParam(l)
			box := g.tracking
			inside := g.boxAt(POINT{x, y}) == box
			g.tracking, g.pressed = boxNone, boxNone
			call(procReleaseCapture)
			g.invalidate()
			if inside {
				if box == boxClose {
					g.performClose()
				} else {
					g.toggleCompact()
				}
			}
		}
		return 0
	case WM_CAPTURECHANGED:
		if g.tracking != boxNone {
			g.tracking, g.pressed = boxNone, boxNone
			g.invalidate()
		}
		return 0
	case WM_COMMAND:
		g.command(int(loword(w)))
		return 0
	case WM_CLOSE:
		g.performClose()
		return 0
	case WM_TIMER:
		// The time markers and the time left move on even when nothing
		// new has been read, so draw again now and then.
		if w == timerRedraw && g.visible() {
			g.render()
		}
		return 0
	case WM_DPICHANGED:
		g.dpi = uint32(hiword(w))
		g.setWindowIcons()
		r := rectAt(l)
		call(procSetWindowPos, uintptr(hwnd), 0, uintptr(r.Left), uintptr(r.Top), 0, 0, SWP_NOZORDER|SWP_NOACTIVATE|SWP_NOSIZE)
		g.chromeKeyReset()
		g.render()
		g.updateTray()
		return 0
	case WM_EXITSIZEMOVE:
		g.savePosition()
		return 0
	case WM_POWERBROADCAST:
		switch w {
		case PBT_APMSUSPEND:
			g.ctrl.NotePower("sleep")
		case PBT_APMRESUMEAUTOMATIC:
			g.ctrl.NotePower("wake")
		}
		return 1
	case WM_CTLCOLORBTN:
		// The corners around the contents' buttons, in the contents' colour.
		return g.contentBrush
	case wmTray:
		g.trayEvent(loword(l))
		return 0
	case WM_QUERYENDSESSION:
		return 1
	case WM_ENDSESSION:
		if w != 0 {
			g.shutdown()
		}
		return 0
	case WM_DESTROY:
		call(procPostQuitMessage, 0)
		return 0
	}
	if msg == g.showMessage && g.showMessage != 0 {
		g.show()
		return 0
	}
	if msg == g.taskbarCreated && g.taskbarCreated != 0 {
		// Explorer restarted: the tray icon is gone with it.
		if g.tray {
			g.tray = false
			g.addTray()
		}
		return 0
	}
	return call(procDefWindowProcW, uintptr(hwnd), uintptr(msg), w, l)
}

var procScreenToClient = user32.NewProc("ScreenToClient")

func (g *gui) invalidate() {
	call(procInvalidateRect, uintptr(g.hwnd), 0, 0)
}

func (g *gui) visible() bool {
	return call(procIsWindowVisible, uintptr(g.hwnd)) != 0
}

// forcedLook is the look to draw in for a snapshot: active unless the
// snapshot asks otherwise.
func (g *gui) forcedLook() bool {
	if g.snapshotLook != "" {
		return g.snapshotLook == "active"
	}
	return g.active
}

func (g *gui) inTitleBar(pt POINT) bool {
	return pt.Y >= 0 && pt.Y < g.lay.s.px(g.state.Theme.TitleBarHeight)
}

func inRect(pt POINT, r RECT) bool {
	return pt.X >= r.Left && pt.X < r.Right && pt.Y >= r.Top && pt.Y < r.Bottom
}

func (g *gui) boxAt(pt POINT) int {
	switch {
	case inRect(pt, g.lay.closeBox):
		return boxClose
	case inRect(pt, g.lay.shadeBox):
		return boxShade
	}
	return boxNone
}

func (g *gui) mouseDown(pt POINT) {
	if box := g.boxAt(pt); box != boxNone {
		// Like a button of the classic Mac OS, a box acts when the mouse is
		// released over it, and looks pressed while held down on it.
		g.tracking, g.pressed = box, box
		call(procSetCapture, uintptr(g.hwnd))
		g.invalidate()
		return
	}
	for i, m := range g.lay.menus {
		if inRect(pt, m.rect) {
			g.popUpMenu(i)
			return
		}
	}
}

// --- menus ----------------------------------------------------------------------

func (g *gui) T(key string) string { return text(g.state, key) }

// menuItem is the text of a menu item with its shortcut, which Windows shows
// after a tab.
func (g *gui) menuItem(key, shortcut string) string {
	if shortcut == "" {
		return g.T(key)
	}
	return g.T(key) + "\t" + shortcut
}

// buildMenu makes one of the menu bar's menus. It holds what the macOS
// window's menu bar inside the window holds.
func (g *gui) buildMenu(which int) uintptr {
	m := call(procCreatePopupMenu)
	add := func(flags uintptr, id int, s string) {
		call(procAppendMenuW, m, flags, uintptr(id), ptr(utf16Ptr(s)))
	}
	switch which {
	case menuUsage:
		add(MF_STRING, cmdFetch, g.menuItem("menu_fetch_now", "Ctrl+R"))
	case menuView:
		flags := uintptr(MF_STRING)
		if g.compact {
			flags |= MF_CHECKED
		}
		add(flags, cmdCompact, g.menuItem("menu_compact", "Ctrl+Shift+C"))
	case menuSettings:
		add(MF_STRING, cmdSettings, g.menuItem("menu_settings", "Ctrl+,"))
	}
	return m
}

// popUpMenu opens a menu bar item's menu below it, highlighting the item
// while it is open.
func (g *gui) popUpMenu(i int) {
	m := g.buildMenu(g.lay.menus[i].menu)
	defer call(procDestroyMenu, m)
	g.openMenu = i
	g.invalidate()
	call(procUpdateWindow, uintptr(g.hwnd))
	r := g.lay.menus[i].rect
	pt := POINT{r.Left, r.Bottom}
	call(procClientToScreen, uintptr(g.hwnd), uintptr(unsafe.Pointer(&pt)))
	cmd := call(procTrackPopupMenuEx, m, TPM_LEFTALIGN|TPM_TOPALIGN|TPM_RETURNCMD|TPM_NONOTIFY,
		uintptr(pt.X), uintptr(pt.Y), uintptr(g.hwnd), 0)
	g.openMenu = -1
	g.invalidate()
	if cmd != 0 {
		g.command(int(cmd))
	}
}

var procUpdateWindow = user32.NewProc("UpdateWindow")

func (g *gui) command(id int) {
	switch id {
	case cmdFetch:
		g.ctrl.FetchNow("manual")
	case cmdCompact:
		g.toggleCompact()
	case cmdSettings:
		g.showSettings()
	case cmdClose:
		g.performClose()
	case cmdShow:
		g.show()
	case cmdQuit:
		g.quit()
	default:
		if i := id - cmdButton; i >= 0 && i < len(g.buttonActs) {
			g.act(g.buttonActs[i])
		}
	}
}

func (g *gui) act(a action) {
	switch a {
	case actionLogin:
		g.openLogin()
	case actionSettings:
		g.showSettings()
	case actionTrust:
		g.openTrust()
	}
}

// --- showing, closing and quitting -----------------------------------------------

func (g *gui) show() {
	if g.hidden {
		return
	}
	was := g.visible()
	if call(procIsIconic, uintptr(g.hwnd)) != 0 {
		call(procShowWindow, uintptr(g.hwnd), SW_RESTORE)
	} else {
		call(procShowWindow, uintptr(g.hwnd), SW_SHOW)
	}
	call(procSetForegroundWindow, uintptr(g.hwnd))
	if !was {
		g.ctrl.NoteWindow("show")
	}
}

func (g *gui) hide() {
	if g.visible() {
		g.savePosition()
		call(procShowWindow, uintptr(g.hwnd), SW_HIDE)
		g.ctrl.NoteWindow("close")
	}
}

// performClose decides, by the close_action setting, whether closing the
// window only hides it (the application keeps running in the notification
// area), quits the application, or asks which.
func (g *gui) performClose() {
	switch g.state.Settings.CloseAction {
	case settings.CloseWindow:
		g.hide()
		return
	case settings.CloseQuit:
		g.quit()
		return
	}
	switch g.askClose() {
	case closeChoiceQuit:
		g.quit()
	case closeChoiceClose:
		g.hide()
	}
}

// quit stops the controller (and with it claude), waiting a little for it to
// finish, and ends the message loop.
func (g *gui) quit() {
	if g.quitting {
		return
	}
	g.shutdown()
	if g.settings != nil && g.settings.hwnd != 0 {
		call(procDestroyWindow, uintptr(g.settings.hwnd))
	}
	call(procDestroyWindow, uintptr(g.hwnd))
}

// shutdown is the part of quitting that must happen also when Windows ends
// the session.
func (g *gui) shutdown() {
	if g.quitting {
		return
	}
	g.quitting = true
	g.savePosition()
	g.removeTray()
	if g.stopRun != nil {
		g.stopRun()
		select {
		case <-g.runDone:
		case <-time.After(8 * time.Second):
		}
	}
}

// --- rendering ------------------------------------------------------------------

// chromeKeyReset makes the next render lay the frame out again.
func (g *gui) chromeKeyReset() { g.lay = layout{} }

// render lays the window out for the current state and draws it.
func (g *gui) render() {
	st := g.state
	// The language may have changed: the settings window is rebuilt in the
	// new one when next opened.
	if string(st.Lang) != g.lang {
		g.lang = string(st.Lang)
		g.fonts.setFace(faceFor(g.lang))
		if g.settings != nil {
			g.settings.destroy()
			g.settings = nil
		}
	}
	if !g.compactSettled && st.Strings != nil {
		g.compactSettled = true
		g.compact = st.Settings.StartCompact
	}
	setWindowText(g.hwnd, g.T("window_title"))
	after := HWND_NOTOPMOST
	if st.Settings.AlwaysOnTop {
		after = HWND_TOPMOST
	}
	call(procSetWindowPos, uintptr(g.hwnd), after, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)

	g.lay = g.layOut()
	g.placeButtons()
	call(procSetWindowPos, uintptr(g.hwnd), 0, 0, 0, uintptr(g.lay.width), uintptr(g.lay.height),
		SWP_NOMOVE|SWP_NOZORDER|SWP_NOACTIVATE)
	g.invalidate()
	g.updateAttention()
	g.updateTray()
	if g.settings != nil {
		g.settings.fillInfo()
	}
	if g.snapshotDir != "" {
		g.snapshot()
	}
}

// layOut works out where everything goes.
func (g *gui) layOut() layout {
	st := g.state
	t := st.Theme
	s := g.scale()
	lay := layout{s: s, width: s.px(kWidth), compact: g.compact}
	dc := windows.Handle(call(procGetDC, uintptr(g.hwnd)))
	defer call(procReleaseDC, uintptr(g.hwnd), uintptr(dc))

	box, boxTop, boxSide := s.px(t.BoxSize), s.px(t.BoxTop), s.px(t.BoxSide)
	lay.closeBox = RECT{boxSide, boxTop, boxSide + box, boxTop + box}
	lay.shadeBox = RECT{lay.width - boxSide - box, boxTop, lay.width - boxSide, boxTop + box}

	// The title: centred on the window when it fits, otherwise in the room
	// between the boxes, where it is cut off. The compact view's title
	// says the usage.
	lay.title = g.T("window_title")
	if g.compact {
		lay.title = compactTitle(st)
	}
	if lay.title != "" {
		gap := s.px(t.BoxGap) + s.px(t.TitleGap)
		left, right := lay.closeBox.Right+gap, lay.shadeBox.Left-gap
		tw, th := measure(dc, lay.title, g.fonts.get(s, titleSize(t), t.TitleBold), 0)
		tw = min(tw, max(right-left, 0))
		x := min(max(lay.width/2-tw/2, left), right-tw)
		mid := s.px(t.BoxTop + t.BoxSize/2)
		lay.titleRect = RECT{x, mid - th/2, x + tw, mid - th/2 + th}
	}

	titleBar := s.px(t.TitleBarHeight)
	if g.compact {
		lay.height = titleBar
		return lay
	}

	// The menu bar inside the window.
	x := s.px(t.MenuLeft)
	menuBar := s.px(t.MenuBarHeight)
	for _, m := range []struct {
		key  string
		menu int
	}{{"menu_usage", menuUsage}, {"menu_view", menuView}, {"menu_settings_menu", menuSettings}} {
		name := g.T(m.key)
		w, _ := measure(dc, name, g.fonts.get(s, menuSize(t), false), 0)
		w += 2 * s.px(t.MenuPadding)
		lay.menus = append(lay.menus, menuTitle{name: name, rect: RECT{x, titleBar, x + w, titleBar + menuBar}, menu: m.menu})
		x += w
	}

	// The contents, stacked from the top inside a margin.
	inset := s.px(t.FrameInset)
	top := titleBar + menuBar
	margin := s.px(kMargin)
	textLeft := inset + margin
	textWidth := lay.width - 2*inset - 2*margin
	y := top + margin
	items := buildView(st, viewEnv{
		Now: time.Now(),
		DaysElapse: func(reset time.Time, window time.Duration) (float64, bool) {
			return g.ctrl.DaysElapsed(reset, window, time.Now())
		},
		DaysAhead: func(reset time.Time, window time.Duration) (float64, bool) {
			return g.ctrl.DaysAhead(reset, window, time.Now())
		},
		DaysError: g.ctrl.DaysError(),
	})
	for i, it := range items {
		var r RECT
		switch {
		case it.Button != "":
			bw, _ := measure(dc, it.Text, g.buttonFontFor(s), 0)
			r = RECT{textLeft, y, textLeft + bw + s.px(24), y + s.px(kButtonHeight)}
		case it.Limit != nil:
			_, _, info := g.limitRows(dc, s, it.Limit, textLeft, y, textWidth)
			r = RECT{textLeft, y, textLeft + textWidth, info.Bottom}
		default:
			_, h := measure(dc, it.Text, g.fonts.get(s, it.Size, it.Bold), textWidth)
			r = RECT{textLeft, y, textLeft + textWidth, y + h}
		}
		lay.items = append(lay.items, placedItem{item: it, rect: r})
		y = r.Bottom
		if i < len(items)-1 {
			y += s.px(it.After)
		}
	}
	y += margin
	lay.content = RECT{inset, top, lay.width - inset, y}
	lay.height = y + inset
	return lay
}

func (g *gui) buttonFontFor(s scale) windows.Handle {
	return g.fonts.get(s, 12, false)
}

// placeButtons makes the buttons in the contents, or moves the ones there
// when they are the same.
func (g *gui) placeButtons() {
	var keys []string
	var acts []action
	var rects []RECT
	var texts []string
	for _, pi := range g.lay.items {
		if pi.item.Button != "" {
			keys = append(keys, string(pi.item.Button)+"\x00"+pi.item.Text)
			acts = append(acts, pi.item.Button)
			rects = append(rects, pi.rect)
			texts = append(texts, pi.item.Text)
		}
	}
	font := g.buttonFontFor(g.lay.s)
	same := len(keys) == len(g.buttonKeys) && font == g.buttonFont
	for i := range keys {
		if !same || keys[i] != g.buttonKeys[i] {
			same = false
		}
	}
	if !same {
		for _, b := range g.buttons {
			call(procDestroyWindow, uintptr(b))
		}
		g.buttons = nil
		for i := range keys {
			h := call(procCreateWindowExW, 0, ptr(utf16Ptr("BUTTON")), ptr(utf16Ptr(texts[i])),
				WS_CHILD|WS_VISIBLE|WS_TABSTOP|BS_PUSHBUTTON, 0, 0, 0, 0,
				uintptr(g.hwnd), uintptr(cmdButton+i), uintptr(moduleHandle()), 0)
			sendMessage(windows.HWND(h), WM_SETFONT, uintptr(font), 1)
			g.buttons = append(g.buttons, windows.HWND(h))
		}
		g.buttonKeys, g.buttonActs, g.buttonFont = keys, acts, font
	}
	for i, b := range g.buttons {
		r := rects[i]
		call(procSetWindowPos, uintptr(b), 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top),
			SWP_NOZORDER|SWP_NOACTIVATE)
		show := uintptr(SW_SHOW)
		if g.compact {
			show = SW_HIDE
		}
		call(procShowWindow, uintptr(b), show)
	}
	// The brush for the buttons' corners.
	if c, ok := colorOf(g.state.Theme.Content); ok {
		if g.contentBrush != 0 {
			call(procDeleteObject, g.contentBrush)
		}
		g.contentBrush = call(procCreateSolidBrush, uintptr(c))
	}
}

// paint draws the whole window on dc.
func (g *gui) paint(dc windows.Handle, active bool) {
	p := painter{dc: dc, s: g.lay.s}
	if p.s == 0 {
		return
	}
	g.paintFrame(p, &g.lay, active)
	if !g.lay.compact {
		g.paintContents(p, &g.lay, active)
	}
}

// paintBuffered draws into a bitmap first, so that nothing flickers.
func (g *gui) paintBuffered(dc windows.Handle, active bool) {
	w, h := g.lay.width, g.lay.height
	if w <= 0 || h <= 0 {
		return
	}
	mem := call(procCreateCompatibleDC, uintptr(dc))
	bmp := call(procCreateCompatibleBitmap, uintptr(dc), uintptr(w), uintptr(h))
	old := call(procSelectObject, mem, bmp)
	g.paint(windows.Handle(mem), active)
	call(procBitBlt, uintptr(dc), 0, 0, uintptr(w), uintptr(h), mem, 0, 0, SRCCOPY)
	call(procSelectObject, mem, old)
	call(procDeleteObject, bmp)
	call(procDeleteDC, mem)
}

func (g *gui) toggleCompact() {
	g.compact = !g.compact
	g.compactSettled = true
	if g.compact {
		g.ctrl.NoteWindow("compact")
	} else {
		g.ctrl.NoteWindow("expand")
	}
	g.render()
}

// --- attention: the tray icon's badge and the notification ---------------------

// updateAttention says "not logged in" outside the window too, whichever
// view is shown: a badge on the tray icon, and a notification once each time
// the login is lost. Not being logged in needs the user.
func (g *gui) updateAttention() {
	out := loggedOut(g.state)
	if out != g.trayBadge {
		g.trayBadge = out
		g.updateTrayIcon()
	}
	if out && !g.notifiedLoggedOut {
		g.notifiedLoggedOut = true
		g.notify(g.T("notify_logged_out_title"), g.T("notify_logged_out_body"))
	} else if !out && g.state.Status != "" && g.state.Status != app.StatusPending {
		g.notifiedLoggedOut = false
	}
}

// --- window position ------------------------------------------------------------

// The window's position is kept in window.json in the application's
// directory, as the top-left corner in pixels of the virtual screen.
type savedPosition struct {
	X int32 `json:"x"`
	Y int32 `json:"y"`
}

func (g *gui) positionFile() string { return filepath.Join(g.paths.Base, "window.json") }

func (g *gui) savePosition() {
	if g.hidden || call(procIsIconic, uintptr(g.hwnd)) != 0 || !g.visible() {
		return
	}
	var r RECT
	call(procGetWindowRect, uintptr(g.hwnd), uintptr(unsafe.Pointer(&r)))
	data, _ := json.Marshal(savedPosition{r.Left, r.Top})
	os.MkdirAll(g.paths.Base, 0o755)
	os.WriteFile(g.positionFile(), data, 0o644)
}

// placeInitially puts the window where it was last time, if that is still on
// a screen, else in the middle of the main screen. CUV_WINDOW_AT ("x,y")
// puts it elsewhere, for testing.
func (g *gui) placeInitially() {
	var at savedPosition
	ok := false
	if v := os.Getenv("CUV_WINDOW_AT"); v != "" {
		_, err := fmt.Sscanf(v, "%d,%d", &at.X, &at.Y)
		ok = err == nil
	} else if data, err := os.ReadFile(g.positionFile()); err == nil && json.Unmarshal(data, &at) == nil {
		// Still on a screen? Check the title bar's middle.
		pt := uintptr(uint32(at.X+g.scale().px(kWidth/2))) | uintptr(uint32(at.Y+g.scale().px(8)))<<32
		ok = call(procMonitorFromPoint, pt, MONITOR_DEFAULTTONULL) != 0
	}
	if !ok {
		var mi MONITORINFO
		mi.Size = uint32(unsafe.Sizeof(mi))
		call(procGetMonitorInfoW, call(procMonitorFromPoint, 0, MONITOR_DEFAULTTONEAREST), uintptr(unsafe.Pointer(&mi)))
		w := mi.Work
		at = savedPosition{w.Left + (w.Right-w.Left-g.scale().px(kWidth))/2, w.Top + (w.Bottom-w.Top)/4}
	}
	call(procSetWindowPos, uintptr(g.hwnd), 0, uintptr(at.X), uintptr(at.Y), 0, 0, SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE)
}

// --- logging in and trusting the folder -----------------------------------------

// openLogin opens a console window running "claude auth login", the way to
// log in again.
func (g *gui) openLogin() {
	script := filepath.Join(g.ctrl.WorkDir(), "login.cmd")
	err := func() error {
		path, err := g.ctrl.ClaudePath()
		if err != nil {
			return err
		}
		body := "@echo off\r\n" +
			"rem Written by claude-usage-visualizer to log Claude Code in again.\r\n" +
			"call " + cmdQuote(path) + " auth login\r\n" +
			"pause\r\n"
		return runScript(script, body)
	}()
	g.ctrl.NoteLoginOpened(script, err)
	if err != nil {
		g.alert(fill(g.T("login_failed"), "error", err.Error()))
	}
}

// openTrust opens a console window running the interactive claude in the
// working directory, so that the user can answer its "trust this folder?"
// question once; the login expiry check needs that. A message saying why is
// printed first.
func (g *gui) openTrust() {
	script := filepath.Join(g.ctrl.WorkDir(), "trust.cmd")
	err := func() error {
		path, err := g.ctrl.ClaudePath()
		if err != nil {
			return err
		}
		body := "@echo off\r\n" +
			"rem Written by claude-usage-visualizer to let the user trust its working folder once.\r\n" +
			"chcp 65001 >nul\r\n" +
			"cd /d " + cmdQuote(g.ctrl.WorkDir()) + " || exit /b 1\r\n" +
			"echo " + cmdEcho(g.T("trust_script_message")) + "\r\n" +
			"echo.\r\n" +
			"call " + cmdQuote(path) + " --strict-mcp-config\r\n"
		return runScript(script, body)
	}()
	g.ctrl.NoteTrustOpened(script, err)
	if err != nil {
		g.alert(fill(g.T("trust_failed"), "error", err.Error()))
	}
}

// runScript writes a batch file and runs it in a console window of its own.
func runScript(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}
	comspec := os.Getenv("ComSpec")
	if comspec == "" {
		comspec = "cmd.exe"
	}
	cmd := exec.Command(comspec, "/c", path)
	cmd.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

// cmdQuote quotes a path for a batch file.
func cmdQuote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// cmdEcho escapes text for echo in a batch file.
func cmdEcho(s string) string {
	r := strings.NewReplacer("^", "^^", "&", "^&", "|", "^|", "<", "^<", ">", "^>", "%", "%%", "\r", " ", "\n", " ")
	return r.Replace(s)
}

// alert shows a message over the window.
func (g *gui) alert(msg string) {
	call(procMessageBoxW, uintptr(g.hwnd), ptr(utf16Ptr(msg)), ptr(utf16Ptr(g.T("window_title"))), 0x30) // MB_ICONWARNING
}

// --- snapshots for development ---------------------------------------------------

// snapshot writes the window as the application draws it to PNGs in the
// directory CUV_SNAPSHOT_DIR names, for checking the drawing during
// development: the usage window in both looks (in front and behind), named
// <theme>-<full|compact>-<active|inactive>.png, and the settings window,
// named settings-<language>.png. It needs no window on the screen, so it
// works with CUV_HIDDEN.
func (g *gui) snapshot() {
	for _, look := range []string{"active", "inactive"} {
		g.snapshotLook = look
		mode := "full"
		if g.compact {
			mode = "compact"
		}
		name := fmt.Sprintf("%s-%s-%s.png", g.state.Theme.ID, mode, look)
		writeWindowPNG(g.hwnd, g.lay.width, g.lay.height, filepath.Join(g.snapshotDir, name))
	}
	g.snapshotLook = ""
	sw := g.settingsForSnapshot()
	if sw != nil {
		var r RECT
		call(procGetWindowRect, uintptr(sw.hwnd), uintptr(unsafe.Pointer(&r)))
		writeWindowPNG(sw.hwnd, r.Right-r.Left, r.Bottom-r.Top, filepath.Join(g.snapshotDir, "settings-"+g.lang+".png"))
	}
}

// writeWindowPNG asks a window to draw itself and its children into a
// bitmap, and writes the bitmap.
func writeWindowPNG(hwnd windows.HWND, w, h int32, path string) {
	if w <= 0 || h <= 0 {
		return
	}
	screen := call(procGetDC, 0)
	mem := call(procCreateCompatibleDC, screen)
	call(procReleaseDC, 0, screen)
	hdr := BITMAPINFOHEADER{Size: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), Width: w, Height: -h, Planes: 1, BitCount: 32}
	var bits unsafe.Pointer
	bmp := call(procCreateDIBSection, mem, uintptr(unsafe.Pointer(&hdr)), DIB_RGB_COLORS, uintptr(unsafe.Pointer(&bits)), 0, 0)
	old := call(procSelectObject, mem, bmp)
	sendMessage(hwnd, WM_PRINT, mem, PRF_NONCLIENT|PRF_CLIENT|PRF_CHILDREN|PRF_ERASEBKGND)
	call(procGdiFlush)
	px := unsafe.Slice((*byte)(bits), int(w*h*4))
	img := image.NewNRGBA(image.Rect(0, 0, int(w), int(h)))
	for i := 0; i < int(w*h); i++ {
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = px[i*4+2], px[i*4+1], px[i*4], 255
	}
	call(procSelectObject, mem, old)
	call(procDeleteObject, bmp)
	call(procDeleteDC, mem)
	if f, err := os.Create(path); err == nil {
		png.Encode(f, img)
		f.Close()
	}
}
