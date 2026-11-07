package days

import (
	"math"
	"strings"
	"testing"
	"time"
)

var tokyo = mustLoad("Asia/Tokyo")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

const sample = `# sample
{
	kinds = { work = 1; off = 0; light = 0.5; };
	weekdays = {
		mon = 'work'; tue = 'work'; wed = 'work'; thu = 'work'; fri = 'work';
		sat = 'off'; sun = 'off';
	};
	days = {
		off = [ '2026-11-03', '2026-12-29..2027-01-03' ];
		work = [ '2026-11-08' ];
	};
	past_days = {
		light = [ '2026-10-16' ];
	};
}
`

func at(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 0, 0, 0, tokyo) }

func TestKinds(t *testing.T) {
	c, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	for date, want := range map[string]string{
		"2026-11-02": "work",  // Monday
		"2026-11-03": "off",   // Tuesday, listed as off
		"2026-11-07": "off",   // Saturday
		"2026-11-08": "work",  // Sunday, listed as work
		"2026-12-31": "off",   // inside the range
		"2027-01-03": "off",   // the last day of the range
		"2027-01-04": "work",  // Monday after it
		"2026-10-16": "light", // from past_days
	} {
		d, _ := parseDate(date)
		if got := c.KindOf(d); got != want {
			t.Errorf("%s: %s, want %s", date, got, want)
		}
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestElapsed(t *testing.T) {
	c, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	// A week from Thursday 2026-10-15 07:00 to Thursday 2026-10-22 07:00.
	// Weights: Thu 17/24 (from 7:00), Fri 0.5 (light, in past_days), Sat 0,
	// Sun 0, Mon 1, Tue 1, Wed 1, Thu 7/24.
	start, end := at(2026, 10, 15, 7), at(2026, 10, 22, 7)
	total := 17.0/24 + 0.5 + 3 + 7.0/24
	tests := []struct {
		now  time.Time
		want float64
	}{
		{start, 0},
		{at(2026, 10, 16, 0), (17.0 / 24) / total},
		{at(2026, 10, 17, 0), (17.0/24 + 0.5) / total},
		{at(2026, 10, 19, 0), (17.0/24 + 0.5) / total}, // the weekend adds nothing
		{at(2026, 10, 19, 12), (17.0/24 + 0.5 + 0.5) / total},
		{end, 1},
		{end.Add(time.Hour), 1},
	}
	for _, tt := range tests {
		got, ok := c.Elapsed(start, end, tt.now, tokyo)
		if !ok || !near(got, tt.want) {
			t.Errorf("at %v: %v (%v), want %v", tt.now, got, ok, tt.want)
		}
	}
}

func TestElapsedAcrossDST(t *testing.T) {
	ny := mustLoad("America/New_York")
	c, err := Parse([]byte(`{ kinds = { a = 1; }; weekdays = { mon='a'; tue='a'; wed='a'; thu='a'; fri='a'; sat='a'; sun='a'; }; }`))
	if err != nil {
		t.Fatal(err)
	}
	// 2026-11-01 is 25 hours long in New York (the clock goes back at 2:00),
	// so its 12:00 is 13 real hours into the day. A uniform weight makes
	// elapsed match plain time.
	start := time.Date(2026, 10, 29, 0, 0, 0, 0, ny)
	end := time.Date(2026, 11, 5, 0, 0, 0, 0, ny)
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, ny)
	got, ok := c.Elapsed(start, end, now, ny)
	days := 3 + 13.0/25
	if !ok || !near(got, days/7) {
		t.Errorf("got %v, want %v", got, days/7)
	}
}

func TestNothingToMeasure(t *testing.T) {
	c, err := Parse([]byte(`{ kinds = { off = 0; }; weekdays = { mon='off'; tue='off'; wed='off'; thu='off'; fri='off'; sat='off'; sun='off'; }; }`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Elapsed(at(2026, 10, 15, 7), at(2026, 10, 22, 7), at(2026, 10, 17, 0), tokyo); ok {
		t.Error("a week that weighs nothing gave a value")
	}
}

func TestRejects(t *testing.T) {
	week := "weekdays = { mon='w'; tue='w'; wed='w'; thu='w'; fri='w'; sat='w'; sun='w'; };"
	for name, src := range map[string]string{
		"no kinds":          "{ " + week + " }",
		"negative weight":   "{ kinds = { w = -1; }; " + week + " }",
		"missing weekday":   "{ kinds = { w = 1; }; weekdays = { mon='w'; }; }",
		"unknown weekday":   "{ kinds = { w = 1; }; weekdays = { mon='w'; tue='w'; wed='w'; thu='w'; fri='w'; sat='w'; sun='w'; funday='w'; }; }",
		"unknown kind":      "{ kinds = { w = 1; }; weekdays = { mon='x'; tue='w'; wed='w'; thu='w'; fri='w'; sat='w'; sun='w'; }; }",
		"unknown list kind": "{ kinds = { w = 1; }; " + week + " days = { x = [ '2026-11-03' ]; }; }",
		"bad date":          "{ kinds = { w = 1; }; " + week + " days = { w = [ '2026-11-31' ]; }; }",
		"backwards range":   "{ kinds = { w = 1; }; " + week + " days = { w = [ '2026-11-05..2026-11-03' ]; }; }",
		"huge range":        "{ kinds = { w = 1; }; " + week + " days = { w = [ '2026-01-01..2099-01-01' ]; }; }",
		"two kinds":         "{ kinds = { w = 1; o = 0; }; " + week + " days = { w = [ '2026-11-03' ]; o = [ '2026-11-01..2026-11-05' ]; }; }",
		"past and days":     "{ kinds = { w = 1; o = 0; }; " + week + " days = { w = [ '2026-11-03' ]; }; past_days = { o = [ '2026-11-03' ]; }; }",
		"unknown key":       "{ kinds = { w = 1; }; " + week + " holidays = []; }",
		"bad kind name":     "{ kinds = { '1st' = 1; }; " + week + " }",
	} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSameDateSameKindIsFine(t *testing.T) {
	src := "{ kinds = { o = 0; w = 1; }; weekdays = { mon='w'; tue='w'; wed='w'; thu='w'; fri='w'; sat='o'; sun='o'; }; days = { o = [ '2026-11-03', '2026-11-01..2026-11-05' ]; }; }"
	if _, err := Parse([]byte(src)); err != nil && !strings.Contains(err.Error(), "both") {
		t.Errorf("rejected: %v", err)
	}
}

func TestElapsedAhead(t *testing.T) {
	c, err := Parse([]byte(DefaultText)) // weekdays 9, weekends 1
	if err != nil {
		t.Fatal(err)
	}
	start, end := at(2026, 10, 15, 7), at(2026, 10, 22, 7)
	week := end.Sub(start).Seconds()
	plain := func(now time.Time) float64 { return now.Sub(start).Seconds() / week }

	// Monday: only weekdays are ahead, so it matches plain time.
	mon := at(2026, 10, 19, 10)
	if got, ok := c.ElapsedAhead(start, end, mon, tokyo); !ok || !near(got, plain(mon)) {
		t.Errorf("Monday: %v, want %v", got, plain(mon))
	}

	// Friday noon: the weekend is ahead and counts a ninth, so it is further
	// along than plain time.
	fri := at(2026, 10, 16, 12)
	aheadHours := 12.0 + 24.0/9 + 24.0/9 + 24*3 + 7
	want := 1 - aheadHours*3600/week
	got, ok := c.ElapsedAhead(start, end, fri, tokyo)
	if !ok || !near(got, want) || got <= plain(fri) {
		t.Errorf("Friday: %v, want %v (plain %v)", got, want, plain(fri))
	}

	// At the start and at the end.
	if got, _ := c.ElapsedAhead(start, end, end, tokyo); !near(got, 1) {
		t.Errorf("end: %v", got)
	}
	if got, _ := c.ElapsedAhead(start, end, start.Add(-time.Hour), tokyo); got < 0 || got > 1 {
		t.Errorf("before start: %v", got)
	}

	// Uniform weights: always plain time.
	u, _ := Parse([]byte("{ kinds = { a = 2; }; weekdays = { mon='a'; tue='a'; wed='a'; thu='a'; fri='a'; sat='a'; sun='a'; }; }"))
	if got, _ := u.ElapsedAhead(start, end, fri, tokyo); !near(got, plain(fri)) {
		t.Errorf("uniform: %v, want %v", got, plain(fri))
	}

	// Nothing weighs anything.
	z, _ := Parse([]byte("{ kinds = { o = 0; }; weekdays = { mon='o'; tue='o'; wed='o'; thu='o'; fri='o'; sat='o'; sun='o'; }; }"))
	if _, ok := z.ElapsedAhead(start, end, fri, tokyo); ok {
		t.Error("a value with all weights zero")
	}
}
