// Package theme defines how the usage window looks: the title bar, the menu
// bar inside the window, the frame around the contents and the bars.
//
// The application draws its own title bar on every OS, and the look of it is
// data rather than code: a theme is defined here once, handed to the window
// with the rest of the state as JSON, and each OS's window only draws what
// the theme says. Adding a look means adding a theme here, not changing the
// windows.
//
// The two themes recall the classic Mac OS without copying its pictures:
// Platinum (Mac OS 8) and System 7.
package theme

// ID names a theme. It is also the value of the theme setting.
type ID string

const (
	// Platinum recalls Mac OS 8: a light gray frame with bevelled edges,
	// etched stripes in the title bar. It is the default.
	Platinum ID = "platinum"
	// System7 recalls System 7: white, with thin black stripes and flat
	// black lines.
	System7 ID = "system7"
)

// Default is the theme used unless the setting names another.
const Default = Platinum

// Color is a colour written as "#RRGGBB", or empty for "do not draw".
type Color string

// Look is the part of a theme that differs between the window in front (the
// key window) and the window behind. Behind, the stripes and the boxes stay,
// only fainter, so that a window in the compact view keeps its presence.
type Look struct {
	// Frame fills the window: the title bar, the menu bar and the margin
	// around the contents.
	Frame Color `json:"frame"`
	// Border is the line around the whole window.
	Border Color `json:"border"`
	// BevelLight and BevelDark are the lines just inside the border, on the
	// top and left, and on the bottom and right, that make the frame look
	// raised. Empty means flat.
	BevelLight Color `json:"bevel_light"`
	BevelDark  Color `json:"bevel_dark"`
	// Stripes are the colours of the title bar's rows from Theme.StripeTop
	// down, repeated until Theme.StripeBottom.
	Stripes []Color `json:"stripes"`
	// TitleSeparator is a line along the bottom of the title bar, or empty.
	TitleSeparator Color `json:"title_separator"`
	// The close box and the window-shade box: the fill, the fill while
	// pressed, the outline, the bevel inside the outline (empty for flat)
	// and the mark inside (an X, or two lines).
	BoxFill        Color `json:"box_fill"`
	BoxPressedFill Color `json:"box_pressed_fill"`
	BoxStroke      Color `json:"box_stroke"`
	BoxBevelLight  Color `json:"box_bevel_light"`
	BoxBevelDark   Color `json:"box_bevel_dark"`
	Glyph          Color `json:"glyph"`
	// Title is the colour of the title's text, MenuText of the menu bar's.
	Title    Color `json:"title"`
	MenuText Color `json:"menu_text"`
	// ContentBorder is the line around the contents.
	ContentBorder Color `json:"content_border"`
}

// Theme is a complete look. Sizes are in points, measured from the window's
// top-left corner.
type Theme struct {
	ID ID `json:"id"`

	// TitleBarHeight is the height of the title bar, counting the border
	// at the top. In the compact view, the window is this tall.
	TitleBarHeight float64 `json:"title_bar_height"`
	// StripeTop and StripeBottom bound the rows that get stripes. The
	// stripes run across the title bar, leaving gaps around the boxes and
	// the title.
	StripeTop    float64 `json:"stripe_top"`
	StripeBottom float64 `json:"stripe_bottom"`
	// StripeSide is how far the stripes stay from the window's sides.
	StripeSide float64 `json:"stripe_side"`
	// BoxSize is the side of the square boxes; BoxTop is their top edge and
	// BoxSide how far they are from the window's sides. BoxGap is the gap
	// the stripes leave around a box, GlyphInset how far the mark inside
	// stays from the box's outline.
	BoxSize    float64 `json:"box_size"`
	BoxTop     float64 `json:"box_top"`
	BoxSide    float64 `json:"box_side"`
	BoxGap     float64 `json:"box_gap"`
	GlyphInset float64 `json:"glyph_inset"`
	// TitleGap is the gap the stripes leave on each side of the title.
	TitleGap      float64 `json:"title_gap"`
	TitleFontSize float64 `json:"title_font_size"`
	TitleBold     bool    `json:"title_bold"`

	// The menu bar inside the window, below the title bar: its height, the
	// size of its text, where its first item starts and the space on each
	// side of an item's name.
	MenuBarHeight float64 `json:"menu_bar_height"`
	MenuFontSize  float64 `json:"menu_font_size"`
	MenuLeft      float64 `json:"menu_left"`
	MenuPadding   float64 `json:"menu_padding"`
	// MenuHighlight fills an item whose menu is open; its name is then
	// drawn in MenuHighlightText.
	MenuHighlight     Color `json:"menu_highlight"`
	MenuHighlightText Color `json:"menu_highlight_text"`

	// FrameInset is the frame's width around the contents, on the sides and
	// at the bottom, counting the border. Zero puts the contents' border on
	// the window's border.
	FrameInset float64 `json:"frame_inset"`
	// Content fills the area that holds the bars.
	Content Color `json:"content"`

	// The bars: the empty part, its outline (empty for none), the used part
	// below 70%, from 70%, from 90%, the used part when the value is old,
	// the dots of the equal parts and the marks of the time elapsed.
	BarTrack   Color `json:"bar_track"`
	BarOutline Color `json:"bar_outline"`
	BarFill    Color `json:"bar_fill"`
	BarWarn    Color `json:"bar_warn"`
	BarDanger  Color `json:"bar_danger"`
	BarStale   Color `json:"bar_stale"`
	TickDot    Color `json:"tick_dot"`
	Marker     Color `json:"marker"`

	Active   Look `json:"active"`
	Inactive Look `json:"inactive"`
}

// IDs lists the themes, the default first.
func IDs() []ID {
	return []ID{Platinum, System7}
}

// Get returns the theme with the given ID.
func Get(id ID) (Theme, bool) {
	switch id {
	case Platinum:
		return platinum(), true
	case System7:
		return system7(), true
	}
	return Theme{}, false
}

// platinum is modelled on the windows of Mac OS 8.
func platinum() Theme {
	return Theme{
		ID:             Platinum,
		TitleBarHeight: 22,
		StripeTop:      6,
		StripeBottom:   16,
		StripeSide:     2,
		BoxSize:        13,
		BoxTop:         5,
		BoxSide:        6,
		BoxGap:         5,
		GlyphInset:     3,
		TitleGap:       6,
		TitleFontSize:  12,
		TitleBold:      true,

		MenuBarHeight:     16,
		MenuFontSize:      11,
		MenuLeft:          3,
		MenuPadding:       7,
		MenuHighlight:     "#333399",
		MenuHighlightText: "#FFFFFF",

		FrameInset: 4,
		Content:    "#FFFFFF",

		BarTrack:   "#CCCCCC",
		BarOutline: "#7A7A7A",
		BarFill:    "#7C7CCF",
		BarWarn:    "#E08A1E",
		BarDanger:  "#D03C3C",
		BarStale:   "#A8A8A8",
		TickDot:    "#8A8A8A",
		Marker:     "#111111",

		Active: Look{
			Frame:          "#DDDDDD",
			Border:         "#262626",
			BevelLight:     "#FFFFFF",
			BevelDark:      "#A8A8A8",
			Stripes:        []Color{"#FFFFFF", "#9A9A9A"},
			BoxFill:        "#DDDDDD",
			BoxPressedFill: "#A8A8A8",
			BoxStroke:      "#4A4A4A",
			BoxBevelLight:  "#FFFFFF",
			BoxBevelDark:   "#A8A8A8",
			Glyph:          "#333333",
			Title:          "#111111",
			MenuText:       "#111111",
			ContentBorder:  "#7A7A7A",
		},
		Inactive: Look{
			Frame:          "#E6E6E6",
			Border:         "#6E6E6E",
			Stripes:        []Color{"#F4F4F4", "#C4C4C4"},
			BoxFill:        "#E6E6E6",
			BoxPressedFill: "#C4C4C4",
			BoxStroke:      "#9A9A9A",
			Glyph:          "#9A9A9A",
			Title:          "#333333",
			MenuText:       "#333333",
			ContentBorder:  "#9A9A9A",
		},
	}
}

// system7 is white with thin black lines and nothing raised. System 7 had
// no window-shade box (it came later, from a control panel of System 7.5),
// but this application puts one on the right in every theme, since the
// compact view needs it.
func system7() Theme {
	return Theme{
		ID:             System7,
		TitleBarHeight: 20,
		StripeTop:      4,
		StripeBottom:   15,
		StripeSide:     1,
		BoxSize:        11,
		BoxTop:         4,
		BoxSide:        8,
		BoxGap:         2,
		GlyphInset:     2,
		TitleGap:       6,
		TitleFontSize:  12,
		TitleBold:      true,

		MenuBarHeight:     18,
		MenuFontSize:      12,
		MenuLeft:          4,
		MenuPadding:       7,
		MenuHighlight:     "#000000",
		MenuHighlightText: "#FFFFFF",

		FrameInset: 0,
		Content:    "#FFFFFF",

		BarTrack:   "#FFFFFF",
		BarOutline: "#000000",
		BarFill:    "#808080",
		BarWarn:    "#E08A1E",
		BarDanger:  "#D03C3C",
		BarStale:   "#C8C8C8",
		TickDot:    "#808080",
		Marker:     "#000000",

		Active: Look{
			Frame:          "#FFFFFF",
			Border:         "#000000",
			Stripes:        []Color{"#000000", "#FFFFFF"},
			TitleSeparator: "#000000",
			BoxFill:        "#FFFFFF",
			BoxPressedFill: "#000000",
			BoxStroke:      "#000000",
			Glyph:          "#000000",
			Title:          "#000000",
			MenuText:       "#000000",
			ContentBorder:  "#000000",
		},
		Inactive: Look{
			Frame:          "#FFFFFF",
			Border:         "#555555",
			Stripes:        []Color{"#C8C8C8", "#FFFFFF"},
			TitleSeparator: "#555555",
			BoxFill:        "#FFFFFF",
			BoxPressedFill: "#C8C8C8",
			BoxStroke:      "#AAAAAA",
			Glyph:          "#AAAAAA",
			Title:          "#333333",
			MenuText:       "#333333",
			ContentBorder:  "#555555",
		},
	}
}
