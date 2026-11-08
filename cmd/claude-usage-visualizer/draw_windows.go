//go:build windows

package main

// Drawing the usage window with GDI. The window has no frame from Windows:
// like the macOS window (gui_darwin.m), it draws its own title bar, menu bar
// and frame in the look the theme describes, so that the two look the same.
// Sizes in the theme and here are points, which are device-independent
// pixels (1/96 inch) on Windows; px turns them into pixels for the monitor's
// scale.

import (
	"math"
	"sort"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/theme"
)

const (
	kWidth  = 380 // the whole window, frame included
	kMargin = 12  // inside the contents' border

	// The bar is drawn inside a taller area, so that the time marker can
	// stick out above and below it, with room above for the usage-days
	// triangle and below for the days-ahead triangle.
	kBarHeight      = 10
	kMarkerOverhang = 4
	kTriangleHeight = 8
	kTriangleWidth  = 10
	kBarBottom      = kTriangleHeight + 2 // from the area's bottom
	kBarArea        = kBarBottom + kBarHeight + kTriangleHeight + 2

	kButtonHeight = 26
)

// faceFor is the typeface for text in lang: Yu Gothic UI for Japanese, as
// Windows itself uses; Segoe UI otherwise, since Japanese fonts draw the
// backslash as a yen sign, which reads wrongly in English paths. Windows
// falls back to a Japanese font for any Japanese text in Segoe UI.
func faceFor(lang string) string {
	if lang == "ja" {
		return "Yu Gothic UI"
	}
	return "Segoe UI"
}

// The colours of text by tone, as the macOS window's light appearance shows
// them on the white contents. The theme's colours are fixed and light, so the
// text on them stays dark whatever the system's colours.
var toneColors = map[tone]uint32{
	toneLabel:     rgb(0x26, 0x26, 0x26),
	toneSecondary: rgb(0x80, 0x80, 0x80),
	toneTertiary:  rgb(0xB0, 0xB0, 0xB0),
	toneRed:       rgb(0xE0, 0x32, 0x28),
	toneOrange:    rgb(0xE8, 0x80, 0x00),
}

// colorOf reads "#RRGGBB"; ok is false for an empty or broken value, which
// means "do not draw".
func colorOf(c theme.Color) (uint32, bool) {
	s := string(c)
	if len(s) != 7 || s[0] != '#' {
		return 0, false
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return 0, false
	}
	return rgb(uint8(v>>16), uint8(v>>8), uint8(v)), true
}

// scale is the monitor's scale: pixels per point.
type scale float64

func (s scale) px(v float64) int32 { return int32(math.Round(v * float64(s))) }

// line is the thickness of a one-point line, at least a pixel.
func (s scale) line() int32 {
	if l := s.px(1); l > 1 {
		return l
	}
	return 1
}

// fonts keeps the fonts made so far, by size, weight and scale, in one
// typeface.
type fonts struct {
	face string
	made map[string]windows.Handle
}

// setFace changes the typeface. The fonts made in the old one are not
// deleted, since windows may still be using them; there are only a few.
func (f *fonts) setFace(face string) {
	if face != f.face {
		f.face = face
		f.made = nil
	}
}

func (f *fonts) get(s scale, size float64, bold bool) windows.Handle {
	key := strconv.FormatFloat(size, 'f', 1, 64) + "/" + strconv.FormatBool(bold) + "/" + strconv.FormatFloat(float64(s), 'f', 3, 64)
	if h, ok := f.made[key]; ok {
		return h
	}
	if f.made == nil {
		f.made = map[string]windows.Handle{}
	}
	weight := FW_NORMAL
	if bold {
		weight = FW_BOLD
	}
	h := windows.Handle(call(procCreateFontW, uintptr(-s.px(size)), 0, 0, 0, uintptr(weight), 0, 0, 0,
		DEFAULT_CHARSET, OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS, CLEARTYPE_QUALITY, DEFAULT_PITCH, uintptr(unsafe.Pointer(utf16Ptr(f.face)))))
	f.made[key] = h
	return h
}

// painter draws on a device context.
type painter struct {
	dc windows.Handle
	s  scale
}

func (p painter) fill(c uint32, r RECT) {
	if r.Right <= r.Left || r.Bottom <= r.Top {
		return
	}
	b := call(procCreateSolidBrush, uintptr(c))
	call(procFillRect, uintptr(p.dc), uintptr(unsafe.Pointer(&r)), b)
	call(procDeleteObject, b)
}

// fillTheme fills r with a theme colour, if it is to be drawn.
func (p painter) fillTheme(c theme.Color, r RECT) {
	if v, ok := colorOf(c); ok {
		p.fill(v, r)
	}
}

// frame draws a border of the given thickness just inside r.
func (p painter) frame(c theme.Color, r RECT, t int32) {
	v, ok := colorOf(c)
	if !ok {
		return
	}
	p.fill(v, RECT{r.Left, r.Top, r.Right, r.Top + t})
	p.fill(v, RECT{r.Left, r.Bottom - t, r.Right, r.Bottom})
	p.fill(v, RECT{r.Left, r.Top, r.Left + t, r.Bottom})
	p.fill(v, RECT{r.Right - t, r.Top, r.Right, r.Bottom})
}

// bevel draws the raised edge just inside r: light on the top and left, dark
// on the bottom and right, one line in from the border.
func (p painter) bevel(light, dark theme.Color, r RECT, t int32) {
	w, h := r.Right-r.Left, r.Bottom-r.Top
	p.fillTheme(light, RECT{r.Left + t, r.Top + t, r.Left + w - t, r.Top + 2*t})
	p.fillTheme(light, RECT{r.Left + t, r.Top + t, r.Left + 2*t, r.Top + h - t})
	p.fillTheme(dark, RECT{r.Left + t, r.Top + h - 2*t, r.Left + w - t, r.Top + h - t})
	p.fillTheme(dark, RECT{r.Left + w - 2*t, r.Top + t, r.Left + w - t, r.Top + h - t})
}

func (p painter) polygon(c uint32, pts []POINT) {
	b := call(procCreateSolidBrush, uintptr(c))
	oldB := call(procSelectObject, uintptr(p.dc), b)
	oldP := call(procSelectObject, uintptr(p.dc), call(procGetStockObject, NULL_PEN))
	call(procPolygon, uintptr(p.dc), uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
	call(procSelectObject, uintptr(p.dc), oldP)
	call(procSelectObject, uintptr(p.dc), oldB)
	call(procDeleteObject, b)
}

func (p painter) ellipse(c uint32, r RECT) {
	b := call(procCreateSolidBrush, uintptr(c))
	oldB := call(procSelectObject, uintptr(p.dc), b)
	oldP := call(procSelectObject, uintptr(p.dc), call(procGetStockObject, NULL_PEN))
	// Without a pen, Ellipse leaves out the right and bottom edges.
	call(procEllipse, uintptr(p.dc), uintptr(r.Left), uintptr(r.Top), uintptr(r.Right+1), uintptr(r.Bottom+1))
	call(procSelectObject, uintptr(p.dc), oldP)
	call(procSelectObject, uintptr(p.dc), oldB)
	call(procDeleteObject, b)
}

func (p painter) lineSeg(c uint32, width int32, x0, y0, x1, y1 int32) {
	pen := call(procCreatePen, PS_SOLID, uintptr(width), uintptr(c))
	old := call(procSelectObject, uintptr(p.dc), pen)
	call(procMoveToEx, uintptr(p.dc), uintptr(x0), uintptr(y0), 0)
	call(procLineTo, uintptr(p.dc), uintptr(x1), uintptr(y1))
	call(procSelectObject, uintptr(p.dc), old)
	call(procDeleteObject, pen)
}

// text draws s in r with DrawText's flags.
func (p painter) text(s string, font windows.Handle, c uint32, r RECT, flags uint32) {
	u, _ := windows.UTF16FromString(s)
	old := call(procSelectObject, uintptr(p.dc), uintptr(font))
	call(procSetBkMode, uintptr(p.dc), TRANSPARENT)
	call(procSetTextColor, uintptr(p.dc), uintptr(c))
	call(procDrawTextW, uintptr(p.dc), uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&r)), uintptr(flags|DT_NOPREFIX))
	call(procSelectObject, uintptr(p.dc), old)
}

// measure returns the size of s in font: on one line, or wrapped to width
// when width is positive.
func measure(dc windows.Handle, s string, font windows.Handle, width int32) (int32, int32) {
	u, _ := windows.UTF16FromString(s)
	old := call(procSelectObject, uintptr(dc), uintptr(font))
	defer call(procSelectObject, uintptr(dc), old)
	r := RECT{0, 0, width, 0}
	flags := uint32(DT_CALCRECT | DT_NOPREFIX)
	if width > 0 {
		flags |= DT_WORDBREAK | DT_EDITCONTROL
	} else {
		flags |= DT_SINGLELINE
	}
	if len(u) <= 1 {
		var tm TEXTMETRICW
		call(procGetTextMetricsW, uintptr(dc), uintptr(unsafe.Pointer(&tm)))
		return 0, tm.Height
	}
	call(procDrawTextW, uintptr(dc), uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&r)), uintptr(flags))
	return r.Right - r.Left, r.Bottom - r.Top
}

// --- layout -------------------------------------------------------------------

// layout is where everything goes, in pixels from the window's top-left,
// worked out for the current state, theme and scale.
type layout struct {
	s       scale
	width   int32
	height  int32
	compact bool

	closeBox, shadeBox RECT
	title              string
	titleRect          RECT

	menus []menuTitle

	content RECT // the area inside the frame that holds the items
	items   []placedItem
}

type menuTitle struct {
	name string
	rect RECT
	menu int // which menu it opens
}

type placedItem struct {
	item viewItem
	rect RECT // for a limit, the whole of it: the name row, bar and info
}

// --- painting -----------------------------------------------------------------

// paintFrame draws the frame, the title bar, the boxes and the menu bar.
func (g *gui) paintFrame(p painter, lay *layout, active bool) {
	t := g.state.Theme
	look := t.Inactive
	if active {
		look = t.Active
	}
	s := lay.s
	l := s.line()
	w, h := lay.width, lay.height
	p.fillTheme(look.Frame, RECT{0, 0, w, h})

	// The stripes, with gaps around the boxes and the title.
	type span struct{ from, to int32 }
	boxGap, titleGap := s.px(t.BoxGap), s.px(t.TitleGap)
	gaps := []span{
		{lay.closeBox.Left - boxGap, lay.closeBox.Right + boxGap},
		{lay.shadeBox.Left - boxGap, lay.shadeBox.Right + boxGap},
	}
	if lay.titleRect.Right > lay.titleRect.Left {
		gaps = append(gaps, span{lay.titleRect.Left - titleGap, lay.titleRect.Right + titleGap})
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i].from < gaps[j].from })
	side := s.px(t.StripeSide)
	for y := t.StripeTop; len(look.Stripes) > 0 && y < t.StripeBottom; y++ {
		c, ok := colorOf(look.Stripes[int(y-t.StripeTop)%len(look.Stripes)])
		if !ok {
			continue
		}
		y0, y1 := s.px(y), s.px(y+1)
		x := side
		for _, g := range gaps {
			if g.from > x {
				p.fill(c, RECT{x, y0, min(g.from, w-side), y1})
			}
			x = max(x, g.to)
		}
		if x < w-side {
			p.fill(c, RECT{x, y0, w - side, y1})
		}
	}

	if lay.titleRect.Right > lay.titleRect.Left {
		c, ok := colorOf(look.Title)
		if !ok {
			c = 0
		}
		p.text(lay.title, g.fonts.get(s, titleSize(t), t.TitleBold), c, lay.titleRect,
			DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS)
	}

	titleBar := s.px(t.TitleBarHeight)
	p.fillTheme(look.TitleSeparator, RECT{0, titleBar - l, w, titleBar})
	full := RECT{0, 0, w, h}
	p.bevel(look.BevelLight, look.BevelDark, full, l)
	p.frame(look.Border, full, l)

	g.paintBox(p, lay.closeBox, false, look, g.pressed == boxClose)
	g.paintBox(p, lay.shadeBox, true, look, g.pressed == boxShade)

	for i, m := range lay.menus {
		c, ok := colorOf(look.MenuText)
		if !ok {
			c = 0
		}
		if g.openMenu == i {
			p.fillTheme(t.MenuHighlight, m.rect)
			if hc, ok := colorOf(t.MenuHighlightText); ok {
				c = hc
			}
		}
		r := m.rect
		r.Left += s.px(t.MenuPadding)
		p.text(m.name, g.fonts.get(s, menuSize(t), false), c, r, DT_LEFT|DT_SINGLELINE|DT_VCENTER)
	}
}

func titleSize(t theme.Theme) float64 {
	if t.TitleFontSize > 0 {
		return t.TitleFontSize
	}
	return 12
}

func menuSize(t theme.Theme) float64 {
	if t.MenuFontSize > 0 {
		return t.MenuFontSize
	}
	return 12
}

// paintBox draws the close box (an X inside) or the window-shade box (two
// lines inside), raised, or sunk while pressed.
func (g *gui) paintBox(p painter, r RECT, shade bool, look theme.Look, pressed bool) {
	t := g.state.Theme
	s := p.s
	l := s.line()
	if pressed {
		p.fillTheme(look.BoxPressedFill, r)
	} else {
		p.fillTheme(look.BoxFill, r)
		p.bevel(look.BoxBevelLight, look.BoxBevelDark, r, l)
	}
	p.frame(look.BoxStroke, r, l)

	glyph := look.Glyph
	if pressed && look.BoxPressedFill == look.Glyph {
		glyph = look.BoxFill // keep the mark visible on a dark press
	}
	c, ok := colorOf(glyph)
	if !ok {
		c = 0
	}
	in := s.px(t.GlyphInset)
	w, h := r.Right-r.Left, r.Bottom-r.Top
	if !shade {
		pen := max(1, s.px(1.2))
		// LineTo leaves out the last pixel; go one further.
		p.lineSeg(c, pen, r.Left+in, r.Top+in, r.Right-in, r.Bottom-in)
		p.lineSeg(c, pen, r.Right-in-1, r.Top+in, r.Left+in-1, r.Bottom-in)
		return
	}
	mid := r.Top + h/2
	p.fill(c, RECT{r.Left + in, mid - l - l/2, r.Left + w - in, mid - l/2})
	p.fill(c, RECT{r.Left + in, mid + l - l/2, r.Left + w - in, mid + 2*l - l/2})
}

// paintContents draws the contents: their background and border, the text
// and the limits. Buttons are child windows, which draw themselves.
func (g *gui) paintContents(p painter, lay *layout, active bool) {
	t := g.state.Theme
	look := t.Inactive
	if active {
		look = t.Active
	}
	s := lay.s
	p.fillTheme(t.Content, lay.content)
	p.frame(look.ContentBorder, lay.content, s.line())

	for _, pi := range lay.items {
		it := pi.item
		switch {
		case it.Limit != nil:
			g.paintLimit(p, it.Limit, pi.rect)
		default:
			// A button, or text in an edit control (see copy_windows.go):
			// child windows that draw themselves.
		}
	}
}

// limitRows works out the parts of a limit: the name row, the bar area and
// the info line, each as a rectangle in r's coordinates.
func (g *gui) limitRows(dc windows.Handle, s scale, lv *limitView, x, y, width int32) (name, bar, info RECT) {
	f := g.fonts.get(s, 13, true)
	_, nh := measure(dc, lv.Label+lv.Percent, f, 0)
	name = RECT{x, y, x + width, y + nh}
	y = name.Bottom + s.px(4)
	bar = RECT{x, y, x + width, y + s.px(kBarArea)}
	y = bar.Bottom + s.px(2)
	_, ih := measure(dc, lv.Info, g.fonts.get(s, 11, false), width)
	info = RECT{x, y, x + width, y + ih}
	return
}

func (g *gui) paintLimit(p painter, lv *limitView, r RECT) {
	s := p.s
	name, bar, info := g.limitRows(p.dc, s, lv, r.Left, r.Top, r.Right-r.Left)
	c := toneColors[toneLabel]
	if lv.Dim {
		c = toneColors[toneSecondary]
	}
	bold := g.fonts.get(s, 13, true)
	pw, _ := measure(p.dc, lv.Percent, bold, 0)
	// The percentage at the right edge; the name takes the room left.
	p.text(lv.Percent, bold, c, RECT{name.Right - pw, name.Top, name.Right, name.Bottom}, DT_RIGHT|DT_SINGLELINE)
	p.text(lv.Label, bold, c, RECT{name.Left, name.Top, name.Right - pw - s.px(8), name.Bottom}, DT_LEFT|DT_SINGLELINE|DT_END_ELLIPSIS)
	g.paintBar(p, lv, bar)
	p.text(lv.Info, g.fonts.get(s, 11, false), toneColors[toneSecondary], info, DT_LEFT|DT_WORDBREAK|DT_EDITCONTROL)
}

// paintBar draws a limit's bar in its area. The bar is square, like the
// progress bars of the classic Mac OS.
func (g *gui) paintBar(p painter, lv *limitView, area RECT) {
	t := g.state.Theme
	s := p.s
	width := area.Right - area.Left
	// y from the area's top for a height measured from its bottom, as the
	// macOS view measures it.
	fromBottom := func(v float64) int32 { return area.Bottom - s.px(v) }
	bar := RECT{area.Left, fromBottom(kBarBottom + kBarHeight), area.Right, fromBottom(kBarBottom)}
	xFor := func(f float64) int32 {
		m := math.Min(f, 1)
		x := math.Min(math.Max(float64(width)*m, 1), float64(width)-1)
		return area.Left + int32(math.Round(x))
	}

	track, ok := colorOf(t.BarTrack)
	if !ok {
		track = rgb(0xDD, 0xDD, 0xDD)
	}
	p.fill(track, bar)
	f := math.Min(math.Max(lv.Fraction, 0), 1)
	if f > 0 {
		filled := bar
		filled.Right = bar.Left + max(2, int32(math.Round(float64(width)*f)))
		fc, ok := colorOf(themeColor(t, lv.FillKey))
		if !ok {
			fc = rgb(0x99, 0x99, 0x99)
		}
		p.fill(fc, filled)
	}
	// The ticks go under the outline and the markers, so that both stay
	// readable.
	slit, ok := colorOf(t.Content)
	if !ok {
		slit = rgb(0xFF, 0xFF, 0xFF)
	}
	sw := max(1, s.px(1.5))
	for _, f := range lv.Slits {
		x := bar.Left + int32(math.Round(float64(width)*f)) - sw/2
		p.fill(slit, RECT{x, bar.Top, x + sw, bar.Bottom})
	}
	if _, ok := colorOf(t.BarOutline); ok {
		p.frame(t.BarOutline, bar, s.line())
	}
	dot, ok := colorOf(t.TickDot)
	if !ok {
		dot = rgb(0x8A, 0x8A, 0x8A)
	}
	d := max(2, s.px(2.4))
	for _, f := range lv.Dots {
		cx := bar.Left + int32(math.Round(float64(width)*f))
		cy := bar.Bottom + s.px(3.2)
		p.ellipse(dot, RECT{cx - d/2, cy - d/2, cx - d/2 + d, cy - d/2 + d})
	}

	marker, ok := colorOf(t.Marker)
	if !ok {
		marker = 0
	}
	if lv.Marker >= 0 {
		x := xFor(lv.Marker)
		mw := max(2, s.px(2))
		p.fill(marker, RECT{x - mw/2, fromBottom(kBarBottom + kBarHeight + kMarkerOverhang), x - mw/2 + mw, fromBottom(kBarBottom - kMarkerOverhang)})
	}
	half := s.px(kTriangleWidth / 2)
	clampX := func(x int32) int32 { return min(max(x, area.Left+half), area.Right-half) }
	if lv.DaysMarker >= 0 {
		x := clampX(xFor(lv.DaysMarker))
		tip := fromBottom(kBarBottom + kBarHeight + 1)
		base := fromBottom(kBarBottom + kBarHeight + 1 + kTriangleHeight)
		p.polygon(marker, []POINT{{x, tip}, {x - half, base}, {x + half, base}})
	}
	if lv.AheadMarker >= 0 {
		x := clampX(xFor(lv.AheadMarker))
		tip := fromBottom(kBarBottom - 1)
		base := fromBottom(kBarBottom - 1 - kTriangleHeight)
		p.polygon(marker, []POINT{{x, tip}, {x - half, base}, {x + half, base}})
	}
}

// themeColor returns the theme's colour for a fill key of limitView.
func themeColor(t theme.Theme, key string) theme.Color {
	switch key {
	case "bar_warn":
		return t.BarWarn
	case "bar_danger":
		return t.BarDanger
	case "bar_stale":
		return t.BarStale
	}
	return t.BarFill
}
