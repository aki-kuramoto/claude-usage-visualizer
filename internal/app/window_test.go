package app

import (
	"testing"
	"time"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/usage"
)

func TestWindowOf(t *testing.T) {
	for label, want := range map[string]time.Duration{
		"Current session":            5 * time.Hour,
		"Current week (all models)":  7 * 24 * time.Hour,
		"Current week (Fable)":       7 * 24 * time.Hour,
		"Current week (Sonnet only)": 7 * 24 * time.Hour,
		"Claude Code and Cowork":     0,
	} {
		if got := windowOf(label); got != want {
			t.Errorf("windowOf(%q) = %v, want %v", label, got, want)
		}
	}
}

func TestLimitsOfCarriesWindow(t *testing.T) {
	reset := "2026-10-21T22:00:00Z"
	got := limitsOf(&usage.Norm{Limits: []usage.NormLimit{
		{Label: "Current week (all models)", UsedPct: 92, ResetsAt: &reset},
		{Label: "Something new", UsedPct: 1},
	}})
	if len(got) != 2 || got[0].WindowSeconds != 7*24*3600 || got[0].ResetsAt != reset || got[1].WindowSeconds != 0 {
		t.Errorf("got %+v", got)
	}
}

// The error kinds listed for the display match those kindOf can return, and
// i18n's test lists the same ones.
func TestErrorKinds(t *testing.T) {
	known := map[string]bool{}
	for _, k := range ErrorKinds {
		known[k] = true
	}
	for _, r := range []usage.Reason{usage.ReasonTimeout, usage.ReasonExited, usage.ReasonUnexpectedFormat,
		usage.ReasonNoLimits, usage.ReasonNoTimeZone, usage.ReasonUnknownTimeZone, usage.ReasonNotFound,
		usage.ReasonStartFailed, usage.ReasonResultError, usage.ReasonNotLoggedIn, "something new"} {
		if k := kindOf(r); !known[k] {
			t.Errorf("kindOf(%q) = %q, not in ErrorKinds", r, k)
		}
	}
}

func TestShortLabel(t *testing.T) {
	s := map[string]string{"compact_session": "5h", "compact_week": "week"}
	for label, want := range map[string]string{
		"Current session":            "5h",
		"Current week (all models)":  "week",
		"Current week (Fable)":       "Fable",
		"Current week (Sonnet only)": "Sonnet",
		"Something new":              "Something new",
	} {
		if got := shortLabel(label, s); got != want {
			t.Errorf("shortLabel(%q) = %q, want %q", label, got, want)
		}
	}
}

func TestTicks(t *testing.T) {
	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	near := func(a, b float64) bool { return a-b < 1e-9 && b-a < 1e-9 }
	same := func(got, want []float64) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if !near(got[i], want[i]) {
				return false
			}
		}
		return true
	}

	// The week from Thursday 07:00 JST: midnights 17 hours in, then every
	// 24 hours; sevenths at 07:00 each day.
	cal, eq := ticks("2026-10-21T22:00:00Z", 7*24*time.Hour, tokyo)
	var wantCal []float64
	for h := 17.0; h < 168; h += 24 {
		wantCal = append(wantCal, h/168)
	}
	if !same(cal, wantCal) || !same(eq, []float64{1.0 / 7, 2.0 / 7, 3.0 / 7, 4.0 / 7, 5.0 / 7, 6.0 / 7}) {
		t.Errorf("week: calendar %v equal %v", cal, eq)
	}

	// Five hours ending 17:10 JST, so from 12:10: hours at 13:00 ... 17:00,
	// fifths at 13:10 ... 16:10.
	cal, eq = ticks("2026-10-19T08:10:00Z", 5*time.Hour, tokyo)
	wantCal = nil
	for m := 50.0; m < 300; m += 60 {
		wantCal = append(wantCal, m/300)
	}
	if !same(cal, wantCal) || !same(eq, []float64{0.2, 0.4, 0.6, 0.8}) {
		t.Errorf("five hours: calendar %v equal %v", cal, eq)
	}

	// A period that starts exactly at midnight gets no tick at its start.
	cal, _ = ticks("2026-10-22T15:00:00Z", 7*24*time.Hour, tokyo) // ends at midnight JST
	if len(cal) != 6 || !near(cal[0], 1.0/7) {
		t.Errorf("midnight start: %v", cal)
	}

	if cal, eq := ticks("", 5*time.Hour, tokyo); cal != nil || eq != nil {
		t.Errorf("no reset time: %v %v", cal, eq)
	}
}
