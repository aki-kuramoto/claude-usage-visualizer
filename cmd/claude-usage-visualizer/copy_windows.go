//go:build windows

package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Copying the window's contents: Ctrl+C, or Copy in the Usage menu, puts
// the selected text on the clipboard when some is selected, and all of the
// contents otherwise (see copyText).
//
// The plain text of the contents, everything but the buttons and the limits,
// is shown in read-only edit controls rather than drawn, so that it can be
// selected with the mouse; their own right-click menu copies too.

var (
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procSetClipboardData = user32.NewProc("SetClipboardData")
	procGetFocus         = user32.NewProc("GetFocus")
	procGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	procGlobalFree       = kernel32.NewProc("GlobalFree")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002

	emGetSel      = 0x00B0
	emSetMargins  = 0x00D3
	ecLeftMargin  = 0x1
	ecRightMargin = 0x2
	wmCopy        = 0x0301
)

// copy puts the selection, or else all of the contents, on the clipboard.
func (g *gui) copy() {
	focus := windows.HWND(call(procGetFocus))
	if _, ok := g.textTones[focus]; ok {
		var from, to uint32
		sendMessage(focus, emGetSel, uintptr(unsafe.Pointer(&from)), uintptr(unsafe.Pointer(&to)))
		if from != to {
			sendMessage(focus, wmCopy, 0, 0)
			return
		}
	}
	setClipboardText(g.hwnd, copyText(g.T("window_title"), g.viewItems()))
}

// setClipboardText replaces what is on the clipboard with s.
func setClipboardText(owner windows.HWND, s string) {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return
	}
	size := uintptr(len(u) * 2)
	mem := call(procGlobalAlloc, gmemMoveable, size)
	if mem == 0 {
		return
	}
	p := call(procGlobalLock, mem)
	if p == 0 {
		call(procGlobalFree, mem)
		return
	}
	// The memory is Windows' (GlobalAlloc), not Go's, so it never moves.
	copy(unsafe.Slice((*uint16)(unsafe.Add(unsafe.Pointer(nil), p)), len(u)), u)
	call(procGlobalUnlock, mem)
	if call(procOpenClipboard, uintptr(owner)) == 0 {
		call(procGlobalFree, mem)
		return
	}
	call(procEmptyClipboard)
	if call(procSetClipboardData, cfUnicodeText, mem) == 0 {
		// The clipboard did not take it, so it is still ours to free.
		call(procGlobalFree, mem)
	}
	call(procCloseClipboard)
}

// placeTexts makes the edit controls for the contents' text, or moves the
// ones there when the text is the same, so that a selection survives the
// window being drawn again.
func (g *gui) placeTexts() {
	type text struct {
		item viewItem
		rect RECT
		font windows.Handle
	}
	var texts []text
	var keys []string
	for _, pi := range g.lay.items {
		it := pi.item
		if it.Button != "" || it.Limit != nil {
			continue
		}
		f := g.fonts.get(g.lay.s, it.Size, it.Bold)
		texts = append(texts, text{it, pi.rect, f})
		keys = append(keys, fmt.Sprintf("%s\x00%d\x00%d", it.Text, it.Tone, f))
	}
	same := len(keys) == len(g.textKeys)
	for i := range keys {
		if same && keys[i] != g.textKeys[i] {
			same = false
		}
	}
	if !same {
		for _, h := range g.texts {
			call(procDestroyWindow, uintptr(h))
		}
		g.texts = nil
		g.textTones = map[windows.HWND]tone{}
		for _, t := range texts {
			h := windows.HWND(call(procCreateWindowExW, 0, uintptr(unsafe.Pointer(utf16Ptr("EDIT"))), uintptr(unsafe.Pointer(utf16Ptr(editText(t.item.Text)))),
				WS_CHILD|ES_MULTILINE|ES_READONLY, 0, 0, 0, 0,
				uintptr(g.hwnd), 0, uintptr(moduleHandle()), 0))
			sendMessage(h, WM_SETFONT, uintptr(t.font), 0)
			// No margins, so that the text sits where it was measured.
			sendMessage(h, emSetMargins, ecLeftMargin|ecRightMargin, 0)
			g.texts = append(g.texts, h)
			g.textTones[h] = t.item.Tone
		}
		g.textKeys = keys
	}
	for i, h := range g.texts {
		r := texts[i].rect
		call(procSetWindowPos, uintptr(h), 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top),
			SWP_NOZORDER|SWP_NOACTIVATE)
		show := uintptr(SW_SHOW)
		if g.compact {
			show = SW_HIDE
		}
		call(procShowWindow, uintptr(h), show)
	}
}

// editText gives s the line ends an edit control shows as such.
func editText(s string) string {
	var b []rune
	for _, r := range s {
		if r == '\n' && (len(b) == 0 || b[len(b)-1] != '\r') {
			b = append(b, '\r')
		}
		b = append(b, r)
	}
	return string(b)
}

// textColors answers WM_CTLCOLORSTATIC for the contents' text: its tone's
// colour on the contents' background. ok is false for other controls.
func (g *gui) textColors(dc uintptr, ctl windows.HWND) (brush uintptr, ok bool) {
	tn, ok := g.textTones[ctl]
	if !ok {
		return 0, false
	}
	call(procSetTextColor, dc, uintptr(toneColors[tn]))
	if c, ok := colorOf(g.state.Theme.Content); ok {
		call(procSetBkColor, dc, uintptr(c))
	}
	return g.contentBrush, true
}
