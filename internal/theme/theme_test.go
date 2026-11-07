package theme

import (
	"reflect"
	"regexp"
	"testing"
)

var hex = regexp.MustCompile(`^#[0-9A-F]{6}$`)

// optional lists the colours that may be empty (meaning "do not draw").
var optional = map[string]bool{
	"BevelLight": true, "BevelDark": true, "TitleSeparator": true,
	"BoxBevelLight": true, "BoxBevelDark": true, "BarOutline": true,
}

// checkColors checks every Color field of v, a Theme or a Look.
func checkColors(t *testing.T, where string, v reflect.Value) {
	t.Helper()
	for i := 0; i < v.NumField(); i++ {
		f, name := v.Field(i), v.Type().Field(i).Name
		switch c := f.Interface().(type) {
		case Color:
			if c == "" && !optional[name] {
				t.Errorf("%s.%s is empty", where, name)
			} else if c != "" && !hex.MatchString(string(c)) {
				t.Errorf("%s.%s = %q, not #RRGGBB", where, name, c)
			}
		case []Color:
			if len(c) == 0 {
				t.Errorf("%s.%s is empty", where, name)
			}
			for _, s := range c {
				if !hex.MatchString(string(s)) {
					t.Errorf("%s.%s has %q, not #RRGGBB", where, name, s)
				}
			}
		case Look:
			checkColors(t, where+"."+name, f)
		}
	}
}

func TestThemes(t *testing.T) {
	ids := IDs()
	if ids[0] != Default {
		t.Errorf("IDs starts with %q, not the default %q", ids[0], Default)
	}
	for _, id := range ids {
		th, ok := Get(id)
		if !ok || th.ID != id {
			t.Errorf("Get(%q) = %q, %v", id, th.ID, ok)
			continue
		}
		where := string(id)
		checkColors(t, where, reflect.ValueOf(th))

		// The boxes and the stripes fit in the title bar, inside the border.
		if th.BoxTop < 1 || th.BoxTop+th.BoxSize > th.TitleBarHeight-1 {
			t.Errorf("%s: a box from %v to %v does not fit a title bar of %v", where, th.BoxTop, th.BoxTop+th.BoxSize, th.TitleBarHeight)
		}
		if th.StripeTop < 1 || th.StripeBottom <= th.StripeTop || th.StripeBottom > th.TitleBarHeight-1 {
			t.Errorf("%s: stripes from %v to %v in a title bar of %v", where, th.StripeTop, th.StripeBottom, th.TitleBarHeight)
		}
		if 2*th.GlyphInset >= th.BoxSize {
			t.Errorf("%s: no room for the mark in a box of %v", where, th.BoxSize)
		}
		for name, v := range map[string]float64{
			"StripeSide": th.StripeSide, "BoxSide": th.BoxSide, "BoxGap": th.BoxGap, "TitleGap": th.TitleGap,
			"TitleFontSize": th.TitleFontSize, "MenuBarHeight": th.MenuBarHeight, "MenuFontSize": th.MenuFontSize,
			"MenuLeft": th.MenuLeft, "MenuPadding": th.MenuPadding,
		} {
			if v <= 0 {
				t.Errorf("%s.%s = %v", where, name, v)
			}
		}
		if th.FrameInset < 0 {
			t.Errorf("%s.FrameInset = %v", where, th.FrameInset)
		}
		// The window in front and the one behind must look different.
		if reflect.DeepEqual(th.Active, th.Inactive) {
			t.Errorf("%s: the inactive look is the active one", where)
		}
	}
	if _, ok := Get("aqua"); ok {
		t.Error("Get accepted an unknown theme")
	}
}
