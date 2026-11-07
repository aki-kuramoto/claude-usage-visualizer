package usage

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// sample is the result text of "claude -p /usage" from Claude Code 2.1.300,
// taken on 2026-10-15, shortened after the analysis heading.
const sample = "You are currently using your subscription to power your Claude Code usage\n" +
	"\n" +
	"Current session: 35% used · resets Oct 15 at 2:20pm (Asia/Tokyo)\n" +
	"Current week (all models): 8% used · resets Oct 22 at 7am (Asia/Tokyo)\n" +
	"Current week (Fable): 0% used · resets Oct 22 at 7am (Asia/Tokyo)\n" +
	"\n" +
	"What's contributing to your limits usage?\n" +
	"Approximate, based on local sessions on this machine — does not include other devices or claude.ai. Behaviors are independent characteristics, not a breakdown.\n" +
	"\n" +
	"Last 24h · 2526 requests · 9 sessions\n" +
	"  83% of your usage was at >150k context\n" +
	"  Top subagents: general-purpose 40%, Explore 5%"

var tokyo = mustLoad("Asia/Tokyo")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

func TestFromTextSample(t *testing.T) {
	at := time.Date(2026, 10, 15, 12, 37, 45, 0, tokyo)
	rec := FromText("01a13ee5-a63b-7c1e-9d2a-5f8e0b7a4c31", MethodStreamJSONResident, at, sample)
	got, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"v":1,"id":"01a13ee5-a63b-7c1e-9d2a-5f8e0b7a4c31","m":"` + string(MethodStreamJSONResident) + `",` +
		`"at":"` + FormatAt(at) + `","ok":true,` +
		`"raw":{"source":"You are currently using your subscription to power your Claude Code usage","limits":[` +
		`{"label":"Current session","usage":"35% used","resets_at":"Oct 15 at 2:20pm (Asia/Tokyo)"},` +
		`{"label":"Current week (all models)","usage":"8% used","resets_at":"Oct 22 at 7am (Asia/Tokyo)"},` +
		`{"label":"Current week (Fable)","usage":"0% used","resets_at":"Oct 22 at 7am (Asia/Tokyo)"}]},` +
		`"norm":{"source":"subscription","limits":[` +
		`{"label":"Current session","used_pct":35,"resets_at":"2026-10-15T05:20:00Z"},` +
		`{"label":"Current week (all models)","used_pct":8,"resets_at":"2026-10-21T22:00:00Z"},` +
		`{"label":"Current week (Fable)","used_pct":0,"resets_at":"2026-10-21T22:00:00Z"}]},` +
		`"error":null}`
	if string(got) != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestFromTextCRLF(t *testing.T) {
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, tokyo)
	crlf := "You are currently using your subscription to power your Claude Code usage\r\n\r\n" +
		"Current session: 35% used · resets Oct 15 at 2:20pm (Asia/Tokyo)\r\n"
	rec := FromText("id", MethodStreamJSONResident, at, crlf)
	if !rec.OK || len(rec.Norm.Limits) != 1 || *rec.Raw.Limits[0].ResetsAt != "Oct 15 at 2:20pm (Asia/Tokyo)" {
		t.Errorf("CRLF text not read cleanly: %+v", rec)
	}
}

func TestNormalizeReset(t *testing.T) {
	ny := mustLoad("America/New_York")
	tests := []struct {
		text string
		at   time.Time
		want string
	}{
		{"Oct 15 at 2:20pm (Asia/Tokyo)", time.Date(2026, 10, 15, 12, 0, 0, 0, tokyo), "2026-10-15T05:20:00Z"},
		{"Oct 22 at 7am (Asia/Tokyo)", time.Date(2026, 10, 15, 12, 0, 0, 0, tokyo), "2026-10-21T22:00:00Z"},
		// The form of Claude Code 2.1.319: a comma instead of "at".
		{"Nov 8, 11:30am (Asia/Tokyo)", time.Date(2026, 11, 8, 8, 5, 0, 0, tokyo), "2026-11-08T02:30:00Z"},
		{"Nov 9, 5pm (Asia/Tokyo)", time.Date(2026, 11, 8, 8, 5, 0, 0, tokyo), "2026-11-09T08:00:00Z"},
		// The reset falls in the next year.
		{"Jan 2 at 7am (Asia/Tokyo)", time.Date(2026, 12, 30, 12, 0, 0, 0, tokyo), "2027-01-01T22:00:00Z"},
		// Read just after New Year, a reset printed before it was in the old year.
		{"Dec 31 at 11pm (Asia/Tokyo)", time.Date(2027, 1, 1, 0, 10, 0, 0, tokyo), "2026-12-31T14:00:00Z"},
		// Time only: today, or tomorrow when that is closer.
		{"2:20pm (Asia/Tokyo)", time.Date(2026, 10, 15, 12, 0, 0, 0, tokyo), "2026-10-15T05:20:00Z"},
		{"1am (Asia/Tokyo)", time.Date(2026, 10, 15, 23, 30, 0, 0, tokyo), "2026-10-15T16:00:00Z"},
		{"12am (Asia/Tokyo)", time.Date(2026, 10, 15, 20, 0, 0, 0, tokyo), "2026-10-15T15:00:00Z"},
		{"12pm (Asia/Tokyo)", time.Date(2026, 10, 15, 8, 0, 0, 0, tokyo), "2026-10-15T03:00:00Z"},
		// Another zone, in daylight saving time.
		{"Oct 22 at 7am (America/New_York)", time.Date(2026, 10, 15, 12, 0, 0, 0, ny), "2026-10-22T11:00:00Z"},
		// The reading time's own zone does not matter, only the printed one.
		{"Oct 22 at 7am (Asia/Tokyo)", time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC), "2026-10-21T22:00:00Z"},
	}
	for _, tt := range tests {
		got, err := normalizeReset(tt.text, tt.at)
		if err != nil {
			t.Errorf("%q at %v: %+v", tt.text, tt.at, err)
			continue
		}
		if s := got.UTC().Format(time.RFC3339); s != tt.want {
			t.Errorf("%q at %v = %s, want %s", tt.text, tt.at, s, tt.want)
		}
	}
}

func TestNormalizeResetRejects(t *testing.T) {
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, tokyo)
	tests := []struct {
		text   string
		reason Reason
	}{
		{"Oct 22 at 7am", ReasonNoTimeZone},
		{"Oct 22 at 7am (Mars/Olympus)", ReasonUnknownTimeZone},
		{"in 3 hours (Asia/Tokyo)", ReasonUnexpectedFormat},
		{"Oct 22 at 13pm (Asia/Tokyo)", ReasonUnexpectedFormat},
		{"Feb 30 at 7am (Asia/Tokyo)", ReasonUnexpectedFormat},
		{"Foo 3 at 7am (Asia/Tokyo)", ReasonUnexpectedFormat},
		{"Nov 8 11:30am (Asia/Tokyo)", ReasonUnexpectedFormat},
	}
	for _, tt := range tests {
		_, err := normalizeReset(tt.text, at)
		if err == nil || err.Stage != StageNormalize || err.Reason != tt.reason {
			t.Errorf("%q: got %+v, want reason %s", tt.text, err, tt.reason)
		}
	}
}

func TestFromTextKeepsRawWhenNormalizeFails(t *testing.T) {
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, tokyo)
	text := "You are currently using your subscription to power your Claude Code usage\n\n" +
		"Current session: 35% used · resets Oct 15 at 2:20pm\n"
	rec := FromText("id", MethodStreamJSONResident, at, text)
	if rec.OK || rec.Raw == nil || rec.Norm != nil || rec.Error == nil {
		t.Fatalf("got %+v", rec)
	}
	if rec.Error.Stage != StageNormalize || rec.Error.Reason != ReasonNoTimeZone {
		t.Errorf("error = %+v", rec.Error)
	}
}

func TestFromTextOverage(t *testing.T) {
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, tokyo)
	text := "You are currently using your overages to power your Claude Code usage. We will automatically switch you back to your subscription rate limits when they reset\n\n" +
		"Current session: 100% used · resets Oct 15 at 2:20pm (Asia/Tokyo)\n"
	rec := FromText("id", MethodStreamJSONResident, at, text)
	if !rec.OK || *rec.Norm.Source != SourceOverage {
		t.Errorf("got %+v", rec)
	}
}

func TestFromTextUnknownSource(t *testing.T) {
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, tokyo)
	text := "You are currently using something new\n\nCurrent session: 1% used\n"
	rec := FromText("id", MethodStreamJSONResident, at, text)
	if rec.OK || rec.Raw == nil || rec.Error.Reason != ReasonUnexpectedFormat {
		t.Errorf("got %+v", rec)
	}
}

func TestFromTextNoResetTime(t *testing.T) {
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, tokyo)
	rec := FromText("id", MethodStreamJSONResident, at, "Current week (Fable): 0% used\n")
	if !rec.OK || rec.Raw.Source != nil || rec.Norm.Source != nil || rec.Norm.Limits[0].ResetsAt != nil {
		t.Errorf("got %+v", rec)
	}
}

func TestFromTextNoLimits(t *testing.T) {
	// What an API key user sees: costs, but no plan limits.
	text := "Total cost:            $0.0000\nTotal duration (API):  0s\n"
	rec := FromText("id", MethodStreamJSONResident, time.Now(), text)
	if rec.OK || rec.Raw != nil || rec.Norm != nil || rec.Error.Stage != StageParse || rec.Error.Reason != ReasonNoLimits {
		t.Errorf("got %+v", rec)
	}
}

func TestFailedJSON(t *testing.T) {
	at := time.Date(2026, 10, 15, 18, 35, 0, 0, tokyo)
	rec := Failed("id", MethodStreamJSONResident, at,
		Error{Stage: StageWait, Reason: ReasonTimeout, Detail: "no result event within 60s"})
	got, _ := json.Marshal(rec)
	want := fmt.Sprintf(`{"v":1,"id":"id","m":"%s","at":"%s","ok":false,"raw":null,"norm":null,`+
		`"error":{"stage":"wait","reason":"timeout","detail":"no result event within 60s"}}`, MethodStreamJSONResident, FormatAt(at))
	if string(got) != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestFormatAtKeepsOffset(t *testing.T) {
	saved := time.Local
	defer func() { time.Local = saved }()
	time.Local = tokyo
	if got := FormatAt(time.Date(2026, 10, 15, 9, 30, 0, 0, time.UTC)); got != "2026-10-15T18:30:00+09:00" {
		t.Errorf("got %s", got)
	}
	time.Local = time.UTC
	if got := FormatAt(time.Date(2026, 10, 15, 9, 30, 0, 0, time.UTC)); got != "2026-10-15T09:30:00Z" {
		t.Errorf("got %s", got)
	}
}
