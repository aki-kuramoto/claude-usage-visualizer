//go:build windows

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/aki-kuramoto/claude-usage-visualizer/assets/icon"
)

// The window and the notification area take icons made at run time from the
// PNGs drawn by tools/genicon, at the exact size each place wants for the
// monitor's scale, and with a badge when claude is not logged in, like the
// badge on the Dock icon on macOS.

var iconSizes = []int{16, 24, 32, 48, 64, 128, 256, 512, 1024}

// iconImage returns the icon at n x n pixels, scaled down from the nearest
// larger PNG.
func iconImage(n int) *image.NRGBA {
	src := 0
	for _, s := range iconSizes {
		if s >= n {
			src = s
			break
		}
	}
	if src == 0 {
		src = iconSizes[len(iconSizes)-1]
	}
	data, err := icon.PNG.ReadFile(fmt.Sprintf("png/icon-%d.png", src))
	if err != nil {
		return image.NewNRGBA(image.Rect(0, 0, n, n))
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return image.NewNRGBA(image.Rect(0, 0, n, n))
	}
	return scaleDown(img, n)
}

// scaleDown averages the source pixels under each target pixel, in
// premultiplied colour so that the transparent edge does not darken.
func scaleDown(src image.Image, n int) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, n, n))
	fx := float64(b.Dx()) / float64(n)
	fy := float64(b.Dy()) / float64(n)
	for y := 0; y < n; y++ {
		y0, y1 := int(float64(y)*fy), int(math.Ceil(float64(y+1)*fy))
		for x := 0; x < n; x++ {
			x0, x1 := int(float64(x)*fx), int(math.Ceil(float64(x+1)*fx))
			var r, g, bl, a, count float64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					c := color.NRGBAModel.Convert(src.At(b.Min.X+sx, b.Min.Y+sy)).(color.NRGBA)
					al := float64(c.A) / 255
					r += float64(c.R) * al
					g += float64(c.G) * al
					bl += float64(c.B) * al
					a += al
					count++
				}
			}
			if count == 0 || a == 0 {
				continue
			}
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8(math.Round(r / a)), G: uint8(math.Round(g / a)), B: uint8(math.Round(bl / a)),
				A: uint8(math.Round(a / count * 255)),
			})
		}
	}
	return dst
}

// addBadge draws a red disc with a white "!" over the lower right of img.
// Each pixel is sampled 4 x 4 times for smooth edges.
func addBadge(img *image.NRGBA) {
	n := float64(img.Bounds().Dx())
	r := n * 0.29
	cx, cy := n-r, n-r
	bar := func(x, y float64) bool { // the stroke of the "!"
		return math.Abs(x-cx) <= r*0.16 && y >= cy-r*0.62 && y <= cy+r*0.14
	}
	dot := func(x, y float64) bool {
		return math.Hypot(x-cx, y-(cy+r*0.45)) <= r*0.17
	}
	red := color.NRGBA{0xD9, 0x30, 0x25, 0xFF}
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			var disc, white float64
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					px, py := float64(x)+(float64(sx)+0.5)/4, float64(y)+(float64(sy)+0.5)/4
					if math.Hypot(px-cx, py-cy) <= r {
						disc++
						if bar(px, py) || dot(px, py) {
							white++
						}
					}
				}
			}
			if disc == 0 {
				continue
			}
			// The disc over the icon, then the white over the disc.
			c := img.NRGBAAt(x, y)
			c = over(c, red, disc/16)
			c = over(c, color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}, white/16)
			img.SetNRGBA(x, y, c)
		}
	}
}

// over puts top over bottom with the given coverage.
func over(bottom, top color.NRGBA, coverage float64) color.NRGBA {
	ta := coverage * float64(top.A) / 255
	ba := float64(bottom.A) / 255
	a := ta + ba*(1-ta)
	if a == 0 {
		return color.NRGBA{}
	}
	mix := func(t, b uint8) uint8 {
		return uint8(math.Round((float64(t)*ta + float64(b)*ba*(1-ta)) / a))
	}
	return color.NRGBA{mix(top.R, bottom.R), mix(top.G, bottom.G), mix(top.B, bottom.B), uint8(math.Round(a * 255))}
}

// makeIcon turns img into an icon handle, which the caller destroys.
func makeIcon(img *image.NRGBA) windows.Handle {
	n := img.Bounds().Dx()
	hdr := BITMAPINFOHEADER{
		Size: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), Width: int32(n), Height: -int32(n), // top-down
		Planes: 1, BitCount: 32, Compression: BI_RGB,
	}
	var bits unsafe.Pointer
	color := windows.Handle(call(procCreateDIBSection, 0, uintptr(unsafe.Pointer(&hdr)), DIB_RGB_COLORS,
		uintptr(unsafe.Pointer(&bits)), 0, 0))
	if color == 0 {
		return 0
	}
	defer call(procDeleteObject, uintptr(color))
	px := unsafe.Slice((*byte)(bits), n*n*4)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			c := img.NRGBAAt(x, y)
			i := (y*n + x) * 4
			px[i], px[i+1], px[i+2], px[i+3] = c.B, c.G, c.R, c.A
		}
	}
	// The mask is ignored for a 32-bit colour bitmap with alpha, but must
	// be there.
	mask := windows.Handle(call(procCreateBitmap, uintptr(n), uintptr(n), 1, 1, 0))
	defer call(procDeleteObject, uintptr(mask))
	info := ICONINFO{FIcon: 1, Mask: mask, Color: color}
	return windows.Handle(call(procCreateIconIndirect, uintptr(unsafe.Pointer(&info))))
}

// appIcon makes the application icon at n x n pixels, with the badge or not.
func appIcon(n int, badge bool) windows.Handle {
	img := iconImage(n)
	if badge {
		addBadge(img)
	}
	return makeIcon(img)
}
