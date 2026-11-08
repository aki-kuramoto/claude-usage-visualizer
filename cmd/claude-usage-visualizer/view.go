package main

// This file decides what the usage window shows, as a list of items, without
// drawing anything. The Windows window draws the list; the macOS window
// predates it and builds the same contents in gui_darwin.m. It follows that
// file's render method item by item, so the two show the same things.

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/app"
)

// tone is the colour role of a piece of text; each OS picks the colour.
type tone int

const (
	toneLabel     tone = iota // ordinary text
	toneSecondary             // less important: times, explanations
	toneTertiary              // least important: error details
	toneRed                   // something is wrong
	toneOrange                // something needs doing soon
)

// action is what a button in the contents does.
type action string

const (
	actionLogin    action = "login"
	actionSettings action = "settings"
	actionTrust    action = "trust"
)

// defaultSpacing is the gap below an item that sets none of its own.
const defaultSpacing = 6

// viewItem is one row of the contents: text, a button, or a limit (its name
// and percentage, its bar, and a line about its period).
type viewItem struct {
	Text string
	Size float64 // points
	Tone tone
	Bold bool

	// Button, when set, makes the item a button that does it, with Text on it.
	Button action

	Limit *limitView

	// After is the gap below the item, in points.
	After float64
}

// limitView is a limit as the window shows it.
type limitView struct {
	Label   string
	Percent string
	Dim     bool // the value is old: the latest reading failed

	// Fraction is how full the bar is; FillKey names the theme colour of the
	// used part ("bar_fill", "bar_warn", "bar_danger" or "bar_stale").
	Fraction float64
	FillKey  string
	// Marker, DaysMarker and AheadMarker are shares of the period from 0 to
	// 1, or negative for none: the time elapsed (a line across the bar), the
	// same counted by the weights of days.setay (a triangle above) and judged
	// by the days ahead (a triangle below).
	Marker, DaysMarker, AheadMarker float64
	// Slits are where the local calendar turns over; Dots the period's equal
	// parts.
	Slits, Dots []float64

	// Info is the line below the bar: the reset time, the time left and the
	// shares elapsed.
	Info string
}

// viewEnv is what building the view needs besides the state: the time, and
// the controller's answers about days.setay.
type viewEnv struct {
	Now        time.Time
	DaysElapse func(reset time.Time, window time.Duration) (float64, bool)
	DaysAhead  func(reset time.Time, window time.Duration) (float64, bool)
	DaysError  string
}

// text looks a string up in the state's table, falling back to the key.
func text(st app.State, key string) string {
	if s, ok := st.Strings[key]; ok {
		return s
	}
	return key
}

// fill puts value in place of {name} in s.
func fill(s, name, value string) string {
	return strings.ReplaceAll(s, "{"+name+"}", value)
}

// buildView lists the full view's contents for st.
func buildView(st app.State, env viewEnv) []viewItem {
	var items []viewItem
	add := func(it viewItem) {
		if it.After == 0 {
			it.After = defaultSpacing
		}
		items = append(items, it)
	}
	status := st.Status
	if status == "" {
		status = app.StatusPending
	}
	failed := status == app.StatusFailed
	fetching := ""
	if st.Fetching {
		fetching = text(st, "fetching_suffix")
	}
	locale := st.DateLocale

	// The heading says what the bars below mean, so that an old value is
	// never mistaken for the current one, and, after a failure, what went
	// wrong and what to do about it.
	kind := st.ErrorKind
	if kind == "" {
		kind = "other"
	}
	switch {
	case status == app.StatusPending:
		add(viewItem{Text: text(st, "pending") + fetching, Size: 15, Tone: toneSecondary, Bold: true})
	case failed:
		add(viewItem{Text: text(st, "err_"+kind+"_title") + fetching, Size: 15, Tone: toneRed, Bold: true})
		bodyKey := "err_" + kind + "_body"
		if body := text(st, bodyKey); body != "" && body != bodyKey {
			add(viewItem{Text: body, Size: 12, Tone: toneLabel})
		}
		switch kind {
		case "not_logged_in":
			add(viewItem{Text: text(st, "login_button"), Button: actionLogin})
		case "not_found", "start":
			add(viewItem{Text: text(st, "open_settings_button"), Button: actionSettings})
		}
		when := text(st, "failed_never")
		if len(st.Limits) > 0 {
			when = fill(text(st, "stale_note"), "time", formatTime(parseTime(st.LastSuccessAt), locale))
		}
		add(viewItem{Text: when, Size: 12, Tone: toneSecondary})
	default:
		when := fill(text(st, "read_at"), "time", formatTime(parseTime(st.LastSuccessAt), locale)) + fetching
		add(viewItem{Text: when, Size: 12, Tone: toneSecondary})
	}
	if failed && st.Error != "" {
		add(viewItem{Text: st.Error, Size: 11, Tone: toneTertiary})
	}
	// The login's expiry, when the check found something to say.
	if e := st.LoginExpiry; e != nil {
		switch e.Status {
		case "days":
			add(viewItem{Text: fill(text(st, "expiry_days"), "days", fmt.Sprint(e.DaysLeft)), Size: 12, Tone: toneOrange, Bold: true})
			add(viewItem{Text: text(st, "login_button"), Button: actionLogin})
		case "trust_needed":
			add(viewItem{Text: text(st, "expiry_trust"), Size: 11, Tone: toneSecondary})
			add(viewItem{Text: text(st, "trust_button"), Button: actionTrust})
		}
	}
	if env.DaysError != "" {
		add(viewItem{Text: fill(text(st, "days_error"), "error", env.DaysError), Size: 11, Tone: toneRed})
	}
	if len(st.Limits) > 0 && len(items) > 0 {
		items[len(items)-1].After = 14
	}

	for _, l := range st.Limits {
		lv := &limitView{
			Label:       l.Label,
			Percent:     fmt.Sprintf("%d%%", l.UsedPct),
			Dim:         failed,
			Fraction:    float64(l.UsedPct) / 100,
			Marker:      -1,
			DaysMarker:  -1,
			AheadMarker: -1,
			Slits:       l.CalendarTicks,
			Dots:        l.EqualTicks,
		}
		switch {
		case failed:
			lv.FillKey = "bar_stale"
		case l.UsedPct >= 90:
			lv.FillKey = "bar_danger"
		case l.UsedPct >= 70:
			lv.FillKey = "bar_warn"
		default:
			lv.FillKey = "bar_fill"
		}

		reset := parseTime(l.ResetsAt)
		window := time.Duration(l.WindowSeconds) * time.Second
		var left time.Duration
		if l.NotStarted {
			// Nothing of the period has passed: it has not begun.
			lv.Marker = 0
		}
		if !reset.IsZero() {
			left = reset.Sub(env.Now)
			if window > 0 {
				lv.Marker = math.Min(math.Max(1-left.Seconds()/window.Seconds(), 0), 1)
			}
			if env.DaysElapse != nil {
				if f, ok := env.DaysElapse(reset, window); ok {
					lv.DaysMarker = f
				}
			}
			if env.DaysAhead != nil {
				if f, ok := env.DaysAhead(reset, window); ok {
					lv.AheadMarker = f
				}
			}
		}

		var parts []string
		if reset.IsZero() {
			parts = append(parts, text(st, "no_reset"))
		} else {
			parts = append(parts, fill(text(st, "resets_at"), "time", formatTime(reset, locale)))
			if left > 0 {
				parts = append(parts, remaining(st, left))
			}
		}
		pct := func(key string, f float64) {
			if f >= 0 {
				parts = append(parts, fill(text(st, key), "pct", fmt.Sprintf("%.0f", math.Floor(f*100))))
			}
		}
		pct("elapsed", lv.Marker)
		pct("elapsed_days", lv.DaysMarker)
		pct("elapsed_ahead", lv.AheadMarker)
		lv.Info = strings.Join(parts, " · ")
		add(viewItem{Limit: lv, After: 14})
	}
	return items
}

// compactTitle is the title of the compact view: "5h 23%  ·  週 42%" (the
// five-hour limit and the weekly limit for all models), or only that nothing
// could be read, so that old values are never taken for current ones.
func compactTitle(st app.State) string {
	switch st.Status {
	case "", app.StatusPending:
		return text(st, "compact_pending")
	case app.StatusFailed:
		return text(st, "compact_failed")
	}
	var parts []string
	for _, l := range st.Limits {
		if l.Primary {
			parts = append(parts, fmt.Sprintf("%s %d%%", l.Short, l.UsedPct))
		}
	}
	return strings.Join(parts, "  ·  ")
}

// loggedOut tells whether claude is not logged in, which needs the user.
func loggedOut(st app.State) bool {
	return st.Status == app.StatusFailed && st.ErrorKind == "not_logged_in"
}

// remaining says how long until a reset, roughly: days and hours, hours and
// minutes, or minutes.
func remaining(st app.State, d time.Duration) string {
	s := int64(d.Seconds())
	days, h, m := s/86400, s%86400/3600, s%3600/60
	num := func(v int64) string { return fmt.Sprint(v) }
	switch {
	case days > 0:
		return fill(fill(text(st, "remaining_dh"), "d", num(days)), "h", num(h))
	case h > 0:
		return fill(fill(text(st, "remaining_hm"), "h", num(h)), "m", num(m))
	}
	return fill(text(st, "remaining_m"), "m", num(m))
}

// parseTime reads an RFC 3339 time from the state, or gives the zero time.
func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

var (
	jaWeekdays = []string{"日", "月", "火", "水", "木", "金", "土"}
	enWeekdays = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
)

// formatTime shows a time in the machine's local time zone, the way the
// macOS window does for the locale: "10/17(金) 1:30" in Japanese, "Fri,
// 10/17, 1:30 AM" in English.
func formatTime(t time.Time, locale string) string {
	if t.IsZero() {
		return "-"
	}
	t = t.Local()
	if strings.HasPrefix(locale, "ja") {
		return fmt.Sprintf("%d/%d(%s) %d:%02d", t.Month(), t.Day(), jaWeekdays[t.Weekday()], t.Hour(), t.Minute())
	}
	h := t.Hour() % 12
	if h == 0 {
		h = 12
	}
	ampm := "AM"
	if t.Hour() >= 12 {
		ampm = "PM"
	}
	return fmt.Sprintf("%s, %d/%d, %d:%02d %s", enWeekdays[t.Weekday()], t.Month(), t.Day(), h, t.Minute(), ampm)
}

// copyRule is the line that separates the parts of copyText, as in the text
// a Windows message box puts on the clipboard for Ctrl+C.
const copyRule = "---------------------------"

// copyText is the window's contents as plain text, for copying all of it:
// laid out the way a Windows message box copies itself, with the title, the
// text and the buttons between rules. Lines end in CRLF, which every program
// on Windows reads and macOS ones do too.
func copyText(title string, items []viewItem) string {
	var lines []string
	var buttons []string
	for _, it := range items {
		switch {
		case it.Button != "":
			buttons = append(buttons, it.Text)
		case it.Limit != nil:
			l := it.Limit
			// A blank line sets each limit apart from what is above it.
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, l.Label+"\t"+l.Percent)
			if l.Info != "" {
				lines = append(lines, l.Info)
			}
		case it.Text != "":
			lines = append(lines, it.Text)
		}
	}
	out := []string{copyRule, title, copyRule}
	out = append(out, lines...)
	out = append(out, "", copyRule)
	if len(buttons) > 0 {
		out = append(out, strings.Join(buttons, "   ")+"   ", copyRule)
	}
	return strings.Join(out, "\r\n") + "\r\n"
}
