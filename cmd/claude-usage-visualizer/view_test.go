package main

import (
	"strings"
	"testing"
	"time"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/app"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/i18n"
)

func state(lang i18n.Lang) app.State {
	return app.State{Lang: lang, Strings: i18n.Strings(lang), DateLocale: i18n.DateLocale(lang)}
}

func TestFormatTime(t *testing.T) {
	saved := time.Local
	time.Local = time.FixedZone("JST", 9*3600)
	defer func() { time.Local = saved }()
	at := time.Date(2026, 10, 16, 16, 30, 0, 0, time.UTC) // Sat 17 Oct 1:30 JST
	if got := formatTime(at, "ja_JP"); got != "10/17(土) 1:30" {
		t.Errorf("ja: %q", got)
	}
	if got := formatTime(at, "en_US"); got != "Sat, 10/17, 1:30 AM" {
		t.Errorf("en: %q", got)
	}
	noon := time.Date(2026, 10, 17, 3, 5, 0, 0, time.UTC)
	if got := formatTime(noon, "en_US"); got != "Sat, 10/17, 12:05 PM" {
		t.Errorf("en noon: %q", got)
	}
	if got := formatTime(time.Time{}, "en_US"); got != "-" {
		t.Errorf("zero: %q", got)
	}
}

func TestBuildViewOK(t *testing.T) {
	now := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	st := state(i18n.English)
	st.Status = app.StatusOK
	st.LastSuccessAt = now.Format(time.RFC3339)
	st.Limits = []app.Limit{
		{Label: "Current session", UsedPct: 75, ResetsAt: now.Add(time.Hour).Format(time.RFC3339), WindowSeconds: 5 * 3600},
		{Label: "Current week (all models)", UsedPct: 10},
	}
	items := buildView(st, viewEnv{Now: now})
	if len(items) != 3 || items[0].Tone != toneSecondary || items[0].After != 14 {
		t.Fatalf("%+v", items)
	}
	l := items[1].Limit
	if l == nil || l.FillKey != "bar_warn" || l.Percent != "75%" || l.Marker != 0.8 || l.DaysMarker != -1 {
		t.Errorf("%+v", l)
	}
	if !strings.Contains(l.Info, remaining(st, time.Hour)) || !strings.Contains(l.Info, "80") {
		t.Errorf("info %q", l.Info)
	}
	if items[2].Limit.Marker != -1 || !strings.Contains(items[2].Limit.Info, text(st, "no_reset")) {
		t.Errorf("%+v", items[2].Limit)
	}

	// A period that has not begun has its marker at the left edge.
	st.Limits = []app.Limit{{Label: "Current session", WindowSeconds: 5 * 3600, NotStarted: true}}
	items = buildView(st, viewEnv{Now: now})
	if l := items[1].Limit; l.Marker != 0 || !strings.Contains(l.Info, text(st, "no_reset")) {
		t.Errorf("not started: %+v", l)
	}
}

func TestBuildViewLoggedOut(t *testing.T) {
	st := state(i18n.Japanese)
	st.Status = app.StatusFailed
	st.ErrorKind = "not_logged_in"
	st.Error = "spawn / not_logged_in: x"
	items := buildView(st, viewEnv{Now: time.Now()})
	var button action
	for _, it := range items {
		if it.Button != "" {
			button = it.Button
		}
	}
	if items[0].Tone != toneRed || button != actionLogin || items[len(items)-1].Tone != toneTertiary {
		t.Errorf("%+v", items)
	}
	if !loggedOut(st) {
		t.Error("not seen as logged out")
	}
}

func TestCompactTitle(t *testing.T) {
	st := state(i18n.English)
	if compactTitle(st) != text(st, "compact_pending") {
		t.Error(compactTitle(st))
	}
	st.Status = app.StatusOK
	st.Limits = []app.Limit{{Short: "5h", UsedPct: 23, Primary: true}, {Short: "Opus", UsedPct: 5}, {Short: "week", UsedPct: 42, Primary: true}}
	if got := compactTitle(st); got != "5h 23%  ·  week 42%" {
		t.Errorf("%q", got)
	}
}

func TestCopyText(t *testing.T) {
	items := []viewItem{
		{Text: "Not logged in", Bold: true},
		{Text: "Log in again."},
		{Text: "Log in", Button: actionLogin},
		{Limit: &limitView{Label: "Current session", Percent: "0%", Info: "No reset time · 0% of the time elapsed"}},
	}
	want := "---------------------------\r\n" +
		"Claude Code usage\r\n" +
		"---------------------------\r\n" +
		"Not logged in\r\n" +
		"Log in again.\r\n" +
		"\r\n" +
		"Current session\t0%\r\n" +
		"No reset time · 0% of the time elapsed\r\n" +
		"\r\n" +
		"---------------------------\r\n" +
		"Log in   \r\n" +
		"---------------------------\r\n"
	if got := copyText("Claude Code usage", items); got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
	if got := copyText("T", []viewItem{{Text: "a"}}); got != "---------------------------\r\nT\r\n---------------------------\r\na\r\n\r\n---------------------------\r\n" {
		t.Errorf("no buttons: %q", got)
	}
}
