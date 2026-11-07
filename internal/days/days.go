// Package days reads days.setay, which says how much the user expects to use
// Claude on each day, and turns it into how much of a limit's period has
// passed when only the days of use are counted.
//
// The file names kinds of day with a weight each (work = 1; off = 0; ...),
// gives each weekday a kind, and lists exceptions: dates (or ranges of
// dates) of a given kind. Exceptions that are over are moved to past_days by
// the application, so that a missing date can be told apart from one that
// has simply passed. The format is described in docs/days-setay.md.
package days

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	setay "github.com/aki-kuramoto/setay-go"
)

// Weekday names, in the order of time.Weekday (Sunday first).
var weekdayKeys = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// kindName is what a kind may be called: it is used as a key in the file.
var kindName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// maxRangeDays guards against a mistyped range that would span centuries.
const maxRangeDays = 366 * 2

// File is days.setay as written.
type File struct {
	Kinds    map[string]float64  `setay:"kinds"`
	Weekdays map[string]string   `setay:"weekdays"`
	Days     map[string][]string `setay:"days"`
	PastDays map[string][]string `setay:"past_days"`
}

// Calendar is a checked File, ready to answer questions.
type Calendar struct {
	weight  map[string]float64 // kind -> weight
	weekday [7]string          // time.Weekday -> kind
	dates   map[civilDate]string
}

// civilDate is a day on the calendar, without a time or a zone.
type civilDate struct {
	y int
	m time.Month
	d int
}

func dateOf(t time.Time) civilDate {
	y, m, d := t.Date()
	return civilDate{y, m, d}
}

func (c civilDate) String() string { return fmt.Sprintf("%04d-%02d-%02d", c.y, c.m, c.d) }

// start is the beginning of the day in loc.
func (c civilDate) start(loc *time.Location) time.Time {
	return time.Date(c.y, c.m, c.d, 0, 0, 0, 0, loc)
}

func (c civilDate) next() civilDate {
	return dateOf(time.Date(c.y, c.m, c.d+1, 12, 0, 0, 0, time.UTC))
}

func (c civilDate) before(o civilDate) bool {
	if c.y != o.y {
		return c.y < o.y
	}
	if c.m != o.m {
		return c.m < o.m
	}
	return c.d < o.d
}

// Span is one entry of a date list: a single date or an inclusive range.
type Span struct {
	First, Last civilDate
}

// ParseSpan reads "2026-11-03" or "2026-12-29..2027-01-03".
func ParseSpan(s string) (Span, error) {
	first, last, isRange := strings.Cut(s, "..")
	a, err := parseDate(first)
	if err != nil {
		return Span{}, fmt.Errorf("%q: %w", s, err)
	}
	b := a
	if isRange {
		if b, err = parseDate(last); err != nil {
			return Span{}, fmt.Errorf("%q: %w", s, err)
		}
		if b.before(a) {
			return Span{}, fmt.Errorf("%q: the range ends before it starts", s)
		}
		if b.start(time.UTC).Sub(a.start(time.UTC)) > maxRangeDays*24*time.Hour {
			return Span{}, fmt.Errorf("%q: the range is longer than %d days", s, maxRangeDays)
		}
	}
	return Span{a, b}, nil
}

func parseDate(s string) (civilDate, error) {
	t, err := time.Parse(time.DateOnly, strings.TrimSpace(s))
	if err != nil {
		return civilDate{}, errors.New("not a date in the form YYYY-MM-DD")
	}
	return dateOf(t), nil
}

// Parse reads and checks the text of days.setay.
func Parse(data []byte) (*Calendar, error) {
	var f File
	if err := setay.Unmarshal(data, &f, setay.DisallowUnknownFields()); err != nil {
		return nil, err
	}
	return f.check()
}

func (f *File) check() (*Calendar, error) {
	c := &Calendar{weight: map[string]float64{}, dates: map[civilDate]string{}}
	if len(f.Kinds) == 0 {
		return nil, errors.New("kinds is empty: name at least one kind of day with its weight")
	}
	for name, w := range f.Kinds {
		if !kindName.MatchString(name) {
			return nil, fmt.Errorf("kind %q: use letters, digits, _ and -, starting with a letter or _", name)
		}
		if w < 0 || math.IsNaN(w) || math.IsInf(w, 0) {
			return nil, fmt.Errorf("kind %q: the weight must be zero or more, not %v", name, w)
		}
		c.weight[name] = w
	}
	for key := range f.Weekdays {
		if indexOf(weekdayKeys, key) < 0 {
			return nil, fmt.Errorf("weekdays has %q; use %s", key, strings.Join(weekdayKeys[1:], ", ")+", sun")
		}
	}
	for i, key := range weekdayKeys {
		kind, ok := f.Weekdays[key]
		if !ok {
			return nil, fmt.Errorf("weekdays has no %s", key)
		}
		if _, ok := c.weight[kind]; !ok {
			return nil, fmt.Errorf("weekdays.%s is %q, which is not in kinds", key, kind)
		}
		c.weekday[i] = kind
	}
	for _, list := range []struct {
		name  string
		lists map[string][]string
	}{{"days", f.Days}, {"past_days", f.PastDays}} {
		for kind, entries := range list.lists {
			if _, ok := c.weight[kind]; !ok {
				return nil, fmt.Errorf("%s.%s: %q is not in kinds", list.name, kind, kind)
			}
			for _, e := range entries {
				sp, err := ParseSpan(e)
				if err != nil {
					return nil, fmt.Errorf("%s.%s: %w", list.name, kind, err)
				}
				for d := sp.First; !sp.Last.before(d); d = d.next() {
					if other, ok := c.dates[d]; ok && other != kind {
						return nil, fmt.Errorf("%s is listed as both %q and %q", d, other, kind)
					}
					c.dates[d] = kind
				}
			}
		}
	}
	return c, nil
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

// KindOf is the kind of the day d (in loc's calendar).
func (c *Calendar) KindOf(d civilDate) string {
	if k, ok := c.dates[d]; ok {
		return k
	}
	return c.weekday[d.start(time.UTC).Weekday()]
}

// Elapsed tells how much of the period [start, end) has passed at now,
// counting each day by its weight and spreading a day's weight evenly over
// its hours. Days are those of loc's calendar. ok is false when the period
// weighs nothing at all, so that there is nothing to measure against.
func (c *Calendar) Elapsed(start, end, now time.Time, loc *time.Location) (frac float64, ok bool) {
	if !end.After(start) {
		return 0, false
	}
	var total, done float64
	for d := dateOf(start.In(loc)); d.start(loc).Before(end); d = d.next() {
		dayStart, dayEnd := d.start(loc), d.next().start(loc)
		w := c.weight[c.KindOf(d)]
		if w == 0 {
			continue
		}
		length := dayEnd.Sub(dayStart).Seconds() // 23 or 25 hours on a DST change
		from, to := later(dayStart, start), earlier(dayEnd, end)
		if to.After(from) {
			total += w * to.Sub(from).Seconds() / length
		}
		if until := earlier(to, now); until.After(from) {
			done += w * until.Sub(from).Seconds() / length
		}
	}
	if total == 0 {
		return 0, false
	}
	return math.Min(math.Max(done/total, 0), 1), true
}

// ElapsedAhead tells how far into the period [start, end) now is, judged by
// the days still ahead: the time left is counted with each hour weighted by
// its day's weight relative to the heaviest kind (an hour of the heaviest
// kind counts as an hour, an hour of a day weighing a ninth of it as a ninth
// of an hour), and the result is one minus that over the period's length.
//
// When every day ahead is of the heaviest kind, this is the same as the
// plain share of time elapsed. Lighter days ahead make it larger: with less
// use to come, more of the limit may already have been used by now. Unlike
// Elapsed, the days already past do not matter. ok is false when no kind
// weighs anything.
func (c *Calendar) ElapsedAhead(start, end, now time.Time, loc *time.Location) (frac float64, ok bool) {
	if !end.After(start) {
		return 0, false
	}
	var heaviest float64
	for _, w := range c.weight {
		heaviest = math.Max(heaviest, w)
	}
	if heaviest == 0 {
		return 0, false
	}
	from := later(now, start)
	var ahead float64 // seconds, weighted
	for d := dateOf(from.In(loc)); d.start(loc).Before(end); d = d.next() {
		dayStart, dayEnd := d.start(loc), d.next().start(loc)
		a, b := later(dayStart, from), earlier(dayEnd, end)
		if b.After(a) {
			ahead += c.weight[c.KindOf(d)] / heaviest * b.Sub(a).Seconds()
		}
	}
	return math.Min(math.Max(1-ahead/end.Sub(start).Seconds(), 0), 1), true
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// Kinds returns the kind names, sorted, for messages and tests.
func (c *Calendar) Kinds() []string {
	var out []string
	for k := range c.weight {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
