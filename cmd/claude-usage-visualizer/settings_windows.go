//go:build windows

package main

// The settings window, made of Windows' own controls, with the same rows as
// the macOS one. Tab moves between them, Enter saves and Esc cancels.

import (
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/settings"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/theme"
)

const settingsClass = "ClaudeUsageVisualizerSettings"

const (
	settingsWidth   = 480 // points
	settingsPadding = 20
	settingsGap     = 12 // between rows
	rowHeight       = 24
)

// choice is an entry of a drop-down list: the setting's value and its name.
type choice struct{ value, name string }

type settingsWindow struct {
	g    *gui
	hwnd windows.HWND
	dpi  uint32

	labels   map[string]windows.HWND // the row labels, by string key
	interval windows.HWND
	updown   windows.HWND
	unit     windows.HWND
	path     windows.HWND
	onTop    windows.HWND
	compact  windows.HWND
	theme    windows.HWND
	language windows.HWND
	closeAct windows.HWND
	expiry   windows.HWND
	errLabel windows.HWND
	info     windows.HWND
	save     windows.HWND
	cancel   windows.HWND

	themes    []choice
	languages []choice
	closes    []choice
	errText   string
}

var settingsClassRegistered bool

func (g *gui) showSettings() {
	if g.settings == nil {
		g.settings = g.newSettingsWindow()
		if g.settings == nil {
			return
		}
	}
	sw := g.settings
	sw.load()
	sw.center()
	call(procShowWindow, uintptr(sw.hwnd), SW_SHOW)
	call(procSetForegroundWindow, uintptr(sw.hwnd))
	call(procSetFocus, uintptr(sw.interval))
}

// settingsForSnapshot returns the settings window, made and filled but not
// shown if it was not open.
func (g *gui) settingsForSnapshot() *settingsWindow {
	if g.settings == nil {
		g.settings = g.newSettingsWindow()
		if g.settings == nil {
			return nil
		}
		g.settings.load()
	}
	return g.settings
}

func (g *gui) newSettingsWindow() *settingsWindow {
	inst := moduleHandle()
	if !settingsClassRegistered {
		wc := WNDCLASSEXW{
			WndProc:    newCallback(settingsProc),
			Instance:   inst,
			Cursor:     windows.Handle(call(procLoadCursorW, 0, IDC_ARROW)),
			Background: windows.Handle(call(procGetSysColorBrush, COLOR_BTNFACE)),
			ClassName:  utf16Ptr(settingsClass),
			Icon:       g.windowIconBig,
			IconSm:     g.windowIconSmall,
		}
		wc.Size = uint32(unsafe.Sizeof(wc))
		if call(procRegisterClassExW, uintptr(unsafe.Pointer(&wc))) == 0 {
			return nil
		}
		settingsClassRegistered = true
	}
	sw := &settingsWindow{g: g, labels: map[string]windows.HWND{}}
	g.settings = sw // settingsProc finds it here while the window is made
	hwnd := call(procCreateWindowExW, WS_EX_CONTROLPARENT|WS_EX_DLGMODALFRAME, ptr(utf16Ptr(settingsClass)),
		ptr(utf16Ptr(g.T("settings_title"))), WS_CAPTION|WS_SYSMENU|WS_CLIPCHILDREN,
		0, 0, 480, 400, uintptr(g.hwnd), 0, uintptr(inst), 0)
	if hwnd == 0 {
		g.settings = nil
		return nil
	}
	sw.hwnd = windows.HWND(hwnd)
	sw.dpi = uint32(call(procGetDpiForWindow, hwnd))
	if sw.dpi == 0 {
		sw.dpi = 96
	}
	sw.build()
	sw.layOut()
	return sw
}

func (sw *settingsWindow) scale() scale { return scale(float64(sw.dpi) / 96) }

func (sw *settingsWindow) T(key string) string { return sw.g.T(key) }

func (sw *settingsWindow) font() windows.Handle { return sw.g.fonts.get(sw.scale(), 12, false) }

// control makes a child window of the given class.
func (sw *settingsWindow) control(class, text string, style uint32, exStyle uint32, id int) windows.HWND {
	h := windows.HWND(call(procCreateWindowExW, uintptr(exStyle), ptr(utf16Ptr(class)), ptr(utf16Ptr(text)),
		uintptr(WS_CHILD|WS_VISIBLE|style), 0, 0, 0, 0, uintptr(sw.hwnd), uintptr(id), uintptr(moduleHandle()), 0))
	sendMessage(h, WM_SETFONT, uintptr(sw.font()), 0)
	return h
}

func (sw *settingsWindow) label(key string) windows.HWND {
	h := sw.control("STATIC", sw.T(key), SS_LEFT|SS_NOPREFIX, 0, 0)
	sw.labels[key] = h
	return h
}

func (sw *settingsWindow) combo(choices []choice) windows.HWND {
	h := sw.control("COMBOBOX", "", CBS_DROPDOWNLIST|WS_TABSTOP|WS_VSCROLL, 0, 0)
	for _, c := range choices {
		sendMessage(h, CB_ADDSTRING, 0, ptr(utf16Ptr(c.name)))
	}
	return h
}

func (sw *settingsWindow) build() {
	st := sw.g.state
	sw.label("interval_label")
	sw.interval = sw.control("EDIT", "", ES_NUMBER|ES_AUTOHSCROLL|WS_TABSTOP, WS_EX_CLIENTEDGE, 0)
	sendMessage(sw.interval, EM_LIMITTEXT, 2, 0)
	sw.updown = sw.control("msctls_updown32", "", UDS_SETBUDDYINT|UDS_ALIGNRIGHT|UDS_ARROWKEYS|UDS_NOTHOUSANDS, 0, 0)
	sendMessage(sw.updown, WM_USER+105 /* UDM_SETBUDDY */, uintptr(sw.interval), 0)
	sendMessage(sw.updown, UDM_SETRANGE32, settings.MinIntervalMinutes, settings.MaxIntervalMinutes)
	sw.unit = sw.control("STATIC", sw.T("interval_unit"), SS_LEFT|SS_NOPREFIX, 0, 0)

	sw.label("claude_path_label")
	sw.path = sw.control("EDIT", "", ES_AUTOHSCROLL|WS_TABSTOP, WS_EX_CLIENTEDGE, 0)
	sendMessage(sw.path, EM_SETCUEBANNER, 1, ptr(utf16Ptr(sw.T("claude_path_hint"))))

	sw.onTop = sw.control("BUTTON", sw.T("always_on_top"), BS_AUTOCHECKBOX|BS_MULTILINE|WS_TABSTOP, 0, 0)
	sw.compact = sw.control("BUTTON", sw.T("start_compact"), BS_AUTOCHECKBOX|BS_MULTILINE|WS_TABSTOP, 0, 0)

	sw.themes = nil
	for _, id := range st.ThemeIDs {
		sw.themes = append(sw.themes, choice{string(id), sw.T("theme_" + string(id))})
	}
	sw.label("theme_label")
	sw.theme = sw.combo(sw.themes)

	sw.languages = []choice{
		{string(settings.LanguageSystem), sw.T("language_system")},
		{string(settings.LanguageJapanese), nameOr(st.LanguageNames, "ja")},
		{string(settings.LanguageEnglish), nameOr(st.LanguageNames, "en")},
	}
	sw.label("language_label")
	sw.language = sw.combo(sw.languages)

	sw.closes = []choice{
		{string(settings.CloseAsk), sw.T("close_ask")},
		{string(settings.CloseWindow), sw.T("close_close")},
		{string(settings.CloseQuit), sw.T("close_quit")},
	}
	sw.label("close_label")
	sw.closeAct = sw.combo(sw.closes)

	sw.expiry = sw.control("BUTTON", sw.T("login_expiry_check"), BS_AUTOCHECKBOX|BS_MULTILINE|WS_TABSTOP, 0, 0)
	sw.errLabel = sw.control("STATIC", "", SS_LEFT|SS_NOPREFIX, 0, 0)
	// Read-only, so that the paths can be selected and copied.
	sw.info = sw.control("EDIT", "", ES_MULTILINE|ES_READONLY, 0, 0)

	// Windows puts the button that confirms first.
	sw.save = sw.control("BUTTON", sw.T("save"), BS_DEFPUSHBUTTON|WS_TABSTOP, 0, IDOK)
	sw.cancel = sw.control("BUTTON", sw.T("cancel"), BS_PUSHBUTTON|WS_TABSTOP, 0, IDCANCEL)
}

func nameOr(names map[string]string, key string) string {
	if n := names[key]; n != "" {
		return n
	}
	return key
}

// layOut places the controls for the window's scale and sizes the window to
// them.
func (sw *settingsWindow) layOut() {
	s := sw.scale()
	dc := windows.Handle(call(procGetDC, uintptr(sw.hwnd)))
	defer call(procReleaseDC, uintptr(sw.hwnd), uintptr(dc))
	font := sw.font()
	for _, h := range []windows.HWND{sw.interval, sw.updown, sw.unit, sw.path, sw.onTop, sw.compact, sw.theme,
		sw.language, sw.closeAct, sw.expiry, sw.errLabel, sw.info, sw.save, sw.cancel} {
		sendMessage(h, WM_SETFONT, uintptr(font), 0)
	}
	for _, h := range sw.labels {
		sendMessage(h, WM_SETFONT, uintptr(font), 0)
	}

	pad := s.px(settingsPadding)
	width := s.px(settingsWidth)
	inner := width - 2*pad
	rowH := s.px(rowHeight)
	gap := s.px(8)
	y := pad
	place := func(h windows.HWND, x, y, w, ht int32) {
		call(procSetWindowPos, uintptr(h), 0, uintptr(x), uintptr(y), uintptr(w), uintptr(ht), SWP_NOZORDER|SWP_NOACTIVATE)
	}
	textW := func(s string) int32 { w, _ := measure(dc, s, font, 0); return w }
	// labelRow places a row's label and returns where its control starts.
	labelRow := func(key string) int32 {
		lw := textW(sw.T(key))
		_, lh := measure(dc, "A", font, 0)
		place(sw.labels[key], pad, y+(rowH-lh)/2, lw, lh)
		return pad + lw + gap
	}
	comboWidth := func(choices []choice) int32 {
		w := int32(0)
		for _, c := range choices {
			w = max(w, textW(c.name))
		}
		return w + s.px(40)
	}
	checkbox := func(h windows.HWND, key string) {
		_, th := measure(dc, sw.T(key), font, inner-s.px(20))
		ht := max(rowH, th+s.px(4))
		place(h, pad, y, inner, ht)
		y += ht + s.px(settingsGap)
	}

	x := labelRow("interval_label")
	ew := s.px(60)
	place(sw.interval, x, y, ew, rowH)
	// The up-down control attaches itself to the field's right edge when
	// it is told its buddy again.
	sendMessage(sw.updown, WM_USER+105, uintptr(sw.interval), 0)
	var r RECT
	call(procGetWindowRect, uintptr(sw.updown), uintptr(unsafe.Pointer(&r)))
	_, uh := measure(dc, "A", font, 0)
	place(sw.unit, x+ew+gap, y+(rowH-uh)/2, textW(sw.T("interval_unit")), uh)
	y += rowH + s.px(settingsGap)

	x = labelRow("claude_path_label")
	place(sw.path, x, y, pad+inner-x, rowH)
	y += rowH + s.px(settingsGap)

	checkbox(sw.onTop, "always_on_top")
	checkbox(sw.compact, "start_compact")

	for _, row := range []struct {
		key     string
		h       windows.HWND
		choices []choice
	}{{"theme_label", sw.theme, sw.themes}, {"language_label", sw.language, sw.languages}, {"close_label", sw.closeAct, sw.closes}} {
		x = labelRow(row.key)
		// A drop-down list's height includes its open list.
		place(row.h, x, y, min(comboWidth(row.choices), pad+inner-x), s.px(200))
		y += rowH + s.px(settingsGap)
	}

	checkbox(sw.expiry, "login_expiry_check")

	if sw.errText != "" {
		_, eh := measure(dc, sw.errText, font, inner)
		place(sw.errLabel, pad, y, inner, eh)
		call(procShowWindow, uintptr(sw.errLabel), SW_SHOW)
		y += eh + s.px(settingsGap)
	} else {
		call(procShowWindow, uintptr(sw.errLabel), SW_HIDE)
	}

	_, ih := measure(dc, sw.infoText(), font, inner)
	place(sw.info, pad, y, inner, ih+s.px(4))
	y += ih + s.px(4) + s.px(settingsGap) + s.px(4)

	bh := s.px(kButtonHeight)
	bw := max(s.px(88), max(textW(sw.T("save")), textW(sw.T("cancel")))+s.px(24))
	place(sw.cancel, pad+inner-bw, y, bw, bh)
	place(sw.save, pad+inner-2*bw-gap, y, bw, bh)
	y += bh + pad

	// Size the window so that its inside is width x y.
	style := uint32(WS_CAPTION | WS_SYSMENU)
	wr := RECT{0, 0, width, y}
	call(procAdjustWindowRectExForDpi, uintptr(unsafe.Pointer(&wr)), uintptr(style), 0,
		uintptr(WS_EX_CONTROLPARENT|WS_EX_DLGMODALFRAME), uintptr(sw.dpi))
	call(procSetWindowPos, uintptr(sw.hwnd), 0, 0, 0, uintptr(wr.Right-wr.Left), uintptr(wr.Bottom-wr.Top),
		SWP_NOMOVE|SWP_NOZORDER|SWP_NOACTIVATE)
}

var procAdjustWindowRectExForDpi = user32.NewProc("AdjustWindowRectExForDpi")

// center puts the window in the middle of the screen the usage window is on.
func (sw *settingsWindow) center() {
	var mi MONITORINFO
	mi.Size = uint32(unsafe.Sizeof(mi))
	call(procGetMonitorInfoW, call(procMonitorFromWindow, uintptr(sw.g.hwnd), MONITOR_DEFAULTTONEAREST), uintptr(unsafe.Pointer(&mi)))
	var r RECT
	call(procGetWindowRect, uintptr(sw.hwnd), uintptr(unsafe.Pointer(&r)))
	w, h := r.Right-r.Left, r.Bottom-r.Top
	x := mi.Work.Left + (mi.Work.Right-mi.Work.Left-w)/2
	y := mi.Work.Top + (mi.Work.Bottom-mi.Work.Top-h)/2
	call(procSetWindowPos, uintptr(sw.hwnd), 0, uintptr(x), uintptr(y), 0, 0, SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE)
}

// load fills the controls with the current settings.
func (sw *settingsWindow) load() {
	s := sw.g.state.Settings
	interval := s.IntervalMinutes
	if interval == 0 {
		interval = settings.DefaultIntervalMinutes
	}
	sendMessage(sw.updown, UDM_SETPOS32, 0, uintptr(interval))
	setWindowText(sw.interval, strconv.Itoa(interval))
	setWindowText(sw.path, s.ClaudePath)
	check := func(h windows.HWND, on bool) {
		v := uintptr(0)
		if on {
			v = BST_CHECKED
		}
		sendMessage(h, BM_SETCHECK, v, 0)
	}
	check(sw.onTop, s.AlwaysOnTop)
	check(sw.compact, s.StartCompact)
	check(sw.expiry, s.LoginExpiryCheck)
	selectChoice(sw.theme, sw.themes, string(s.Theme))
	selectChoice(sw.language, sw.languages, string(s.Language))
	selectChoice(sw.closeAct, sw.closes, string(s.CloseAction))
	sw.errText = ""
	setWindowText(sw.errLabel, "")
	sw.fillInfo()
	sw.layOut()
}

func selectChoice(h windows.HWND, choices []choice, value string) {
	i := 0
	for j, c := range choices {
		if c.value == value {
			i = j
		}
	}
	sendMessage(h, CB_SETCURSEL, uintptr(i), 0)
}

func selected(h windows.HWND, choices []choice) string {
	i := int(int32(sendMessage(h, CB_GETCURSEL, 0, 0)))
	if i < 0 || i >= len(choices) {
		return choices[0].value
	}
	return choices[i].value
}

func (sw *settingsWindow) infoText() string {
	st := sw.g.state
	dash := func(s string) string {
		if s == "" {
			return "-"
		}
		return s
	}
	return strings.Join([]string{
		fill(sw.T("settings_file"), "path", dash(st.SettingsPath)),
		fill(sw.T("days_file"), "path", dash(st.DaysPath)),
		fill(sw.T("logs_dir"), "path", dash(st.LogsDir)),
	}, "\r\n")
}

func (sw *settingsWindow) fillInfo() {
	setWindowText(sw.info, sw.infoText())
}

func (sw *settingsWindow) saveSettings() {
	s := sw.g.state.Settings
	n, err := strconv.Atoi(strings.TrimSpace(getWindowText(sw.interval)))
	if err != nil {
		n = 0 // rejected by the check in settings.Save, with its message
	}
	s.IntervalMinutes = n
	s.ClaudePath = getWindowText(sw.path)
	s.AlwaysOnTop = sendMessage(sw.onTop, BM_GETCHECK, 0, 0) == BST_CHECKED
	s.StartCompact = sendMessage(sw.compact, BM_GETCHECK, 0, 0) == BST_CHECKED
	s.LoginExpiryCheck = sendMessage(sw.expiry, BM_GETCHECK, 0, 0) == BST_CHECKED
	s.Theme = theme.ID(selected(sw.theme, sw.themes))
	s.Language = settings.Language(selected(sw.language, sw.languages))
	s.CloseAction = settings.CloseAction(selected(sw.closeAct, sw.closes))
	if err := sw.g.ctrl.SaveSettings(s); err != nil {
		sw.errText = fill(sw.T("save_failed"), "error", err.Error())
		setWindowText(sw.errLabel, sw.errText)
		sw.layOut()
		return
	}
	sw.hide()
}

func (sw *settingsWindow) hide() {
	call(procShowWindow, uintptr(sw.hwnd), SW_HIDE)
}

func (sw *settingsWindow) destroy() {
	if sw.hwnd != 0 {
		call(procDestroyWindow, uintptr(sw.hwnd))
		sw.hwnd = 0
	}
}

func settingsProc(hwnd windows.HWND, msg uint32, w, l uintptr) uintptr {
	g := theGUI
	var sw *settingsWindow
	if g != nil {
		sw = g.settings
	}
	if sw == nil || (sw.hwnd != 0 && sw.hwnd != hwnd) {
		return call(procDefWindowProcW, uintptr(hwnd), uintptr(msg), w, l)
	}
	switch msg {
	case WM_COMMAND:
		switch loword(w) {
		case IDOK:
			sw.saveSettings()
			return 0
		case IDCANCEL:
			sw.hide()
			return 0
		}
	case WM_CLOSE:
		sw.hide()
		return 0
	case WM_CTLCOLORSTATIC:
		dc, ctl := w, windows.HWND(l)
		bg := call(procGetSysColor, COLOR_BTNFACE)
		call(procSetBkColor, dc, bg)
		if ctl == sw.errLabel {
			call(procSetTextColor, dc, uintptr(toneColors[toneRed]))
		} else {
			call(procSetTextColor, dc, call(procGetSysColor, COLOR_WINDOWTEXT))
		}
		return call(procGetSysColorBrush, COLOR_BTNFACE)
	case WM_DPICHANGED:
		sw.dpi = uint32(hiword(w))
		r := rectAt(l)
		call(procSetWindowPos, uintptr(hwnd), 0, uintptr(r.Left), uintptr(r.Top), 0, 0, SWP_NOZORDER|SWP_NOACTIVATE|SWP_NOSIZE)
		sw.layOut()
		return 0
	case WM_DESTROY:
		if g.settings == sw {
			g.settings = nil
		}
		return 0
	}
	return call(procDefWindowProcW, uintptr(hwnd), uintptr(msg), w, l)
}
