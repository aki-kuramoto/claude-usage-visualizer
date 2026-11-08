// genicon draws the application icon and writes it in the forms each OS
// wants, into assets/icon:
//
//	icon.svg              the icon on its round plate, filling the canvas (Linux "scalable", and the reference)
//	png/icon-<n>.png      the same at 16 to 1024 pixels (Linux hicolor, Windows)
//	AppIcon.icns          macOS, on a square plate with the usual margin around it,
//	                      the drawing enlarged to nearly fill the plate
//	icon.ico              Windows, 16 to 256 pixels in one file
//
// and the Windows resources of the executable, the icon and the manifest,
// into cmd/claude-usage-visualizer as rsrc_windows_<arch>.syso, which go build
// links in by itself (see syso.go).
//
//	go run ./tools/genicon
//
// AppIcon.icns needs iconutil, which only macOS has; elsewhere it is left as
// it is.
//
// The design: a gauge around a worried face ">_<". The gauge is amber up to
// three quarters and red after that, with little left, which is why the face
// is worried. Two small round ears, in the background color, cover the gauge
// at the top, so that the gauge reads as a head. It suggests a terminal (the
// prompt characters) without using any of Anthropic's marks, and the amber is
// deliberately not Anthropic's clay.
//
// Every shape is defined once (shapes below) in a 100 x 100 space; both the
// SVG and the bitmaps are made from that definition. The bitmaps are drawn
// with signed distance functions, which gives smooth edges with the standard
// library alone.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Colors.
var (
	face   = rgb(0xF4, 0xEE, 0xE3) // the background, a warm off-white
	track  = rgb(0xDC, 0xD3, 0xC3) // the unused part of the gauge
	amber  = rgb(0xE8, 0x91, 0x2D) // the used part
	red    = rgb(0xD9, 0x48, 0x3B) // the part that is nearly used up
	ink    = rgb(0x24, 0x22, 0x1F) // the face
	gaugeR = 32.0                  // radius of the gauge's center line
	gaugeW = 10.0                  // width of the gauge
	inkW   = 5.5                   // width of the face's strokes
)

// The gauge runs clockwise from 135 degrees (lower left) to 405 degrees
// (lower right), in screen coordinates (y down).
const (
	gaugeStart = 135.0
	gaugeEnd   = 405.0
	amberEnd   = 337.6 // three quarters
	redStart   = 330.0 // under the end of the amber, so that the two meet cleanly
	redEnd     = 382.0 // about 92%; the rest is track

	// The ears are circles in the background color whose centers sit a
	// little inside the gauge's center line, just large enough to cover the
	// gauge's width there.
	earAngle = 235.0 // the left ear; the right one is mirrored
	earDist  = 30.0  // from the center of the gauge
	earR     = 7.5
)

// shape is one thing to paint, in order.
type shape interface {
	// dist is the signed distance from (x, y) to the shape's edge, negative
	// inside, in the 100 x 100 space.
	dist(x, y float64) float64
	svg() string
	paint() color.NRGBA
}

// strokes returns the widths of the gauge and the face's strokes and the
// ears' radius. small gives thicker lines, for 24 pixels and below, where
// the normal lines blur into the background; the thicker gauge needs larger
// ears to stay covered.
func strokes(small bool) (gw, iw, er float64) {
	if small {
		return 13, 8.5, 9
	}
	return gaugeW, inkW, earR
}

// shapes returns the icon on a square plate with corners of the given
// radius, filling the 100 x 100 space: the form macOS expects. The drawing
// is enlarged by macScale, so that it nearly fills the plate.
func shapes(radius float64, small bool) []shape {
	ss := drawing(roundRect{0, 0, 100, 100, radius, face}, small)
	k := macScale(small)
	for i := 1; i < len(ss); i++ {
		ss[i] = scaled{ss[i], k}
	}
	return ss
}

// macScale is how much the drawing is enlarged on the macOS plate. As
// drawn, the head fills three quarters of the plate, which left the picture
// small next to other icons. At 1.3 the gauge comes within about 2% of the
// plate's edge. The small variant's thicker gauge would then reach past the
// edge, so it is enlarged only as far as that 2% gap allows.
func macScale(small bool) float64 {
	const k, gap = 1.3, 2.0
	if small {
		gw, _, _ := strokes(true)
		return math.Min(k, (50-gap)/(gaugeR+gw/2))
	}
	return k
}

// headR is the radius of the round plate: just enough for the gauge and
// the ears.
func headR(small bool) float64 {
	gw, _, er := strokes(small)
	return math.Max(gaugeR+gw/2, earDist+er)
}

// roundShapes returns the icon on a round plate, the head, with nothing
// around it: the form for Windows and Linux, whose icons are shapes rather
// than tiles. Drawn with roundMargin, the head fills the canvas.
func roundShapes(small bool) []shape {
	return drawing(circle{50, 50, headR(small), face}, small)
}

// roundPad is the space left around the head, in the 100 x 100 space, so
// that its antialiased edge is not cut off.
const roundPad = 0.5

// roundMargin is render's margin that makes the head fill the canvas. It is
// negative: the 100 x 100 space is larger than the canvas.
func roundMargin(small bool) float64 {
	k := 50 / (headR(small) + roundPad) // how much larger than the canvas
	return (1 - k) / 2
}

// drawing puts the gauge and the face on plate.
func drawing(plate shape, small bool) []shape {
	gw, iw, er := strokes(small)
	t := earAngle * math.Pi / 180
	ex, ey := earDist*math.Cos(t), earDist*math.Sin(t)
	return []shape{
		plate,
		arc{50, 50, gaugeR, gw, gaugeStart, gaugeEnd, track},
		arc{50, 50, gaugeR, gw, redStart, redEnd, red},
		arc{50, 50, gaugeR, gw, gaugeStart, amberEnd, amber},
		circle{50 + ex, 50 + ey, er, face},                    // left ear
		circle{50 - ex, 50 + ey, er, face},                    // right ear
		polyline{[]pt{{34, 43}, {43, 49}, {34, 55}}, iw, ink}, // >
		polyline{[]pt{{44, 64}, {56, 64}}, iw, ink},           // _
		polyline{[]pt{{66, 43}, {57, 49}, {66, 55}}, iw, ink}, // <
	}
}

// smallBelow is the largest size, in pixels of the shape itself, drawn with
// the small variant.
const smallBelow = 24

func main() {
	root, err := moduleRoot()
	if err != nil {
		fail(err)
	}
	out := filepath.Join(root, "assets", "icon")
	if err := os.MkdirAll(filepath.Join(out, "png"), 0o755); err != nil {
		fail(err)
	}

	// The round head filling the canvas, for Windows, Linux and the SVG.
	v := headR(false) + roundPad
	if err := os.WriteFile(filepath.Join(out, "icon.svg"), []byte(svgOf(roundShapes(false), 50-v, 2*v)), 0o644); err != nil {
		fail(err)
	}
	pngs := map[int][]byte{}
	for _, n := range []int{16, 24, 32, 48, 64, 128, 256, 512, 1024} {
		small := n <= smallBelow
		data := encode(render(roundShapes(small), n, roundMargin(small)))
		pngs[n] = data
		if err := os.WriteFile(filepath.Join(out, "png", fmt.Sprintf("icon-%d.png", n)), data, 0o644); err != nil {
			fail(err)
		}
	}
	if err := writeICO(filepath.Join(out, "icon.ico"), pngs, []int{16, 24, 32, 48, 64, 128, 256}); err != nil {
		fail(err)
	}

	if err := writeSysos(filepath.Join(root, "cmd", "claude-usage-visualizer"), pngs, []int{16, 24, 32, 48, 64, 128, 256}); err != nil {
		fail(err)
	}

	// macOS: the shape sits inside the canvas with a margin (Apple's grid
	// puts an 824 pixel shape on a 1024 pixel canvas), with rounder corners.
	if _, err := exec.LookPath("iconutil"); err != nil {
		fmt.Println("==> iconutil not found (it comes with macOS); AppIcon.icns left as it is")
	} else if err := writeICNS(filepath.Join(out, "AppIcon.icns")); err != nil {
		fail(err)
	}
	fmt.Println("==> wrote", out)
}

// render draws the shapes into an n x n image. margin is the fraction of the
// canvas left empty on each side.
func render(ss []shape, n int, margin float64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	scale := 100 / (float64(n) * (1 - 2*margin)) // shape units per pixel
	offset := margin * float64(n)
	for py := 0; py < n; py++ {
		for px := 0; px < n; px++ {
			x := (float64(px) + 0.5 - offset) * scale
			y := (float64(py) + 0.5 - offset) * scale
			var r, g, b, a float64
			for _, s := range ss {
				// Coverage from the distance: a one-pixel-wide ramp across
				// the edge gives the antialiasing.
				cov := clamp(0.5 - s.dist(x, y)/scale)
				if cov == 0 {
					continue
				}
				c := s.paint()
				sa := cov * float64(c.A) / 255
				r = float64(c.R)*sa + r*(1-sa)
				g = float64(c.G)*sa + g*(1-sa)
				b = float64(c.B)*sa + b*(1-sa)
				a = sa + a*(1-sa)
			}
			if a > 0 {
				// r, g, b are premultiplied by a.
				img.SetNRGBA(px, py, color.NRGBA{
					R: uint8(math.Round(r / a)), G: uint8(math.Round(g / a)), B: uint8(math.Round(b / a)),
					A: uint8(math.Round(a * 255)),
				})
			}
		}
	}
	return img
}

// svgOf writes the shapes as SVG, showing the square of side size whose top
// left corner is at (from, from).
func svgOf(ss []shape, from, size float64) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%g %g %g %g" width="100" height="100">`+"\n", from, from, size, size)
	for _, s := range ss {
		b.WriteString("  " + s.svg() + "\n")
	}
	b.WriteString("</svg>\n")
	return b.String()
}

// --- shapes -----------------------------------------------------------------

type pt struct{ x, y float64 }

type roundRect struct {
	x, y, w, h, r float64
	c             color.NRGBA
}

func (s roundRect) dist(x, y float64) float64 {
	cx, cy := s.x+s.w/2, s.y+s.h/2
	qx := math.Abs(x-cx) - (s.w/2 - s.r)
	qy := math.Abs(y-cy) - (s.h/2 - s.r)
	outside := math.Hypot(math.Max(qx, 0), math.Max(qy, 0))
	return outside + math.Min(math.Max(qx, qy), 0) - s.r
}

func (s roundRect) svg() string {
	return fmt.Sprintf(`<rect x="%g" y="%g" width="%g" height="%g" rx="%g" fill="%s"/>`, s.x, s.y, s.w, s.h, s.r, hex(s.c))
}

func (s roundRect) paint() color.NRGBA { return s.c }

// circle is a filled circle.
type circle struct {
	cx, cy, r float64
	c         color.NRGBA
}

func (s circle) dist(x, y float64) float64 { return math.Hypot(x-s.cx, y-s.cy) - s.r }

func (s circle) svg() string {
	return fmt.Sprintf(`<circle cx="%.2f" cy="%.2f" r="%g" fill="%s"/>`, s.cx, s.cy, s.r, hex(s.c))
}

func (s circle) paint() color.NRGBA { return s.c }

// arc is a stroked circular arc with round ends, from angle a0 to a1 degrees
// clockwise (screen coordinates).
type arc struct {
	cx, cy, r, w, a0, a1 float64
	c                    color.NRGBA
}

func (s arc) point(deg float64) pt {
	t := deg * math.Pi / 180
	return pt{s.cx + s.r*math.Cos(t), s.cy + s.r*math.Sin(t)}
}

func (s arc) dist(x, y float64) float64 {
	deg := math.Atan2(y-s.cy, x-s.cx) * 180 / math.Pi
	// Bring the angle into [a0, a0 + 360).
	for deg < s.a0 {
		deg += 360
	}
	for deg >= s.a0+360 {
		deg -= 360
	}
	if deg <= s.a1 {
		return math.Abs(math.Hypot(x-s.cx, y-s.cy)-s.r) - s.w/2
	}
	p0, p1 := s.point(s.a0), s.point(s.a1)
	return math.Min(math.Hypot(x-p0.x, y-p0.y), math.Hypot(x-p1.x, y-p1.y)) - s.w/2
}

func (s arc) svg() string {
	p0, p1 := s.point(s.a0), s.point(s.a1)
	large := 0
	if s.a1-s.a0 > 180 {
		large = 1
	}
	return fmt.Sprintf(`<path d="M%.2f %.2f A%g %g 0 %d 1 %.2f %.2f" fill="none" stroke="%s" stroke-width="%g" stroke-linecap="round"/>`,
		p0.x, p0.y, s.r, s.r, large, p1.x, p1.y, hex(s.c), s.w)
}

func (s arc) paint() color.NRGBA { return s.c }

// polyline is a stroked line with round ends and joins.
type polyline struct {
	pts []pt
	w   float64
	c   color.NRGBA
}

func (s polyline) dist(x, y float64) float64 {
	d := math.Inf(1)
	for i := 0; i+1 < len(s.pts); i++ {
		d = math.Min(d, segDist(x, y, s.pts[i], s.pts[i+1]))
	}
	return d - s.w/2
}

func segDist(x, y float64, a, b pt) float64 {
	dx, dy := b.x-a.x, b.y-a.y
	t := clamp(((x-a.x)*dx + (y-a.y)*dy) / (dx*dx + dy*dy))
	return math.Hypot(x-(a.x+t*dx), y-(a.y+t*dy))
}

func (s polyline) svg() string {
	var p []string
	for i, q := range s.pts {
		cmd := "L"
		if i == 0 {
			cmd = "M"
		}
		p = append(p, fmt.Sprintf("%s%g %g", cmd, q.x, q.y))
	}
	return fmt.Sprintf(`<path d="%s" fill="none" stroke="%s" stroke-width="%g" stroke-linecap="round" stroke-linejoin="round"/>`,
		strings.Join(p, " "), hex(s.c), s.w)
}

func (s polyline) paint() color.NRGBA { return s.c }

// --- file formats -----------------------------------------------------------

// writeICO writes a Windows icon whose images are PNGs, which Windows has
// accepted since Vista.
func writeICO(path string, pngs map[int][]byte, sizes []int) error {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for _, n := range sizes {
		dim := uint8(n)
		if n >= 256 {
			dim = 0 // 0 means 256
		}
		binary.Write(&b, binary.LittleEndian, struct {
			W, H, Colors, Reserved uint8
			Planes, Bits           uint16
			Size, Offset           uint32
		}{dim, dim, 0, 0, 1, 32, uint32(len(pngs[n])), uint32(offset)})
		offset += len(pngs[n])
	}
	for _, n := range sizes {
		b.Write(pngs[n])
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// writeICNS draws the macOS sizes into an .iconset and converts it with
// iconutil, which comes with macOS.
func writeICNS(path string) error {
	dir, err := os.MkdirTemp("", "genicon-*.iconset")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	// iconutil wants the directory name to end in .iconset.
	set := filepath.Join(dir, "AppIcon.iconset")
	if err := os.Mkdir(set, 0o755); err != nil {
		return err
	}
	const margin = (1024 - 824) / 2.0 / 1024
	for _, n := range []int{16, 32, 128, 256, 512} {
		for _, scale := range []int{1, 2} {
			name := fmt.Sprintf("icon_%dx%d.png", n, n)
			if scale == 2 {
				name = fmt.Sprintf("icon_%dx%d@2x.png", n, n)
			}
			px := n * scale
			small := float64(px)*(1-2*margin) <= smallBelow
			if err := os.WriteFile(filepath.Join(set, name), encode(render(shapes(22.5, small), px, margin)), 0o644); err != nil {
				return err
			}
		}
	}
	out, err := exec.Command("iconutil", "-c", "icns", "-o", path, set).CombinedOutput()
	if err != nil {
		return fmt.Errorf("iconutil: %v\n%s", err, out)
	}
	return nil
}

// --- helpers ----------------------------------------------------------------

// scaled is a shape enlarged k times about the center of the 100 x 100
// space.
type scaled struct {
	s shape
	k float64
}

func (s scaled) dist(x, y float64) float64 {
	return s.k * s.s.dist(50+(x-50)/s.k, 50+(y-50)/s.k)
}

func (s scaled) svg() string {
	return fmt.Sprintf(`<g transform="translate(50 50) scale(%g) translate(-50 -50)">%s</g>`, s.k, s.s.svg())
}

func (s scaled) paint() color.NRGBA { return s.s.paint() }

func encode(img image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		fail(err)
	}
	return b.Bytes()
}

func rgb(r, g, b uint8) color.NRGBA { return color.NRGBA{r, g, b, 255} }

func hex(c color.NRGBA) string { return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B) }

func clamp(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func moduleRoot() (string, error) {
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		return "", err
	}
	mod := strings.TrimSpace(string(out))
	if mod == "" || mod == os.DevNull {
		return "", fmt.Errorf("run it inside the claude-usage-visualizer repository")
	}
	return filepath.Dir(mod), nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "genicon:", err)
	os.Exit(1)
}
