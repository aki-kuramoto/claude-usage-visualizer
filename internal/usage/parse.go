package usage

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	// The reset times name an IANA zone such as Asia/Tokyo. Windows has no
	// zone database of its own, so embed one.
	_ "time/tzdata"
)

// behaviorsHeading starts the analysis that follows the limits. Nothing after
// it is a limit line.
const behaviorsHeading = "What's contributing to your limits usage?"

// sourcePrefix starts the first line when the usage is paid by a plan.
const sourcePrefix = "You are currently using "

// limitLine matches "Current session: 35% used · resets Oct 15 at 2:20pm (Asia/Tokyo)".
// The reset part is absent when Claude Code has no reset time for the limit.
var limitLine = regexp.MustCompile(`^(.+?): (\d+% used)(?: · resets (.+))?$`)

// Parse cuts the /usage text into its pieces without changing them.
func Parse(text string) (*Raw, *Error) {
	raw := &Raw{Limits: []RawLimit{}}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i, line := range lines {
		if line == behaviorsHeading {
			break
		}
		if i == 0 && strings.HasPrefix(line, sourcePrefix) {
			s := line
			raw.Source = &s
			continue
		}
		m := limitLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		limit := RawLimit{Label: m[1], Usage: m[2]}
		if m[3] != "" {
			r := m[3]
			limit.ResetsAt = &r
		}
		raw.Limits = append(raw.Limits, limit)
	}
	if len(raw.Limits) == 0 {
		return nil, &Error{Stage: StageParse, Reason: ReasonNoLimits,
			Detail: "no line of the form \"<label>: <n>% used\""}
	}
	return raw, nil
}

// resetTime matches "Oct 15 at 2:20pm (Asia/Tokyo)" and "7am (Asia/Tokyo)",
// and "Nov 8, 11:30am (Asia/Tokyo)", the form Claude Code 2.1.319 (checked
// on 2026-11-08) prints instead of the first.
var resetTime = regexp.MustCompile(`^(?:([A-Z][a-z]{2}) (\d{1,2})(?: at|,) )?(\d{1,2})(?::(\d{2}))?(am|pm)(?: \(([^)]+)\))?$`)

// Normalize converts the pieces into fixed forms. at is when the text was
// read; the reset times carry no year, and some carry no date, so these are
// taken from the reading time.
func Normalize(raw *Raw, at time.Time) (*Norm, *Error) {
	norm := &Norm{Limits: make([]NormLimit, 0, len(raw.Limits))}
	if raw.Source != nil {
		src, err := normalizeSource(*raw.Source)
		if err != nil {
			return nil, err
		}
		norm.Source = &src
	}
	for _, l := range raw.Limits {
		pct, convErr := strconv.Atoi(strings.TrimSuffix(l.Usage, "% used"))
		if convErr != nil {
			return nil, &Error{Stage: StageNormalize, Reason: ReasonUnexpectedFormat,
				Detail: fmt.Sprintf("usage of %q: %q", l.Label, l.Usage)}
		}
		n := NormLimit{Label: l.Label, UsedPct: pct}
		if l.ResetsAt != nil {
			t, err := normalizeReset(*l.ResetsAt, at)
			if err != nil {
				err.Detail = fmt.Sprintf("reset of %q: %s", l.Label, err.Detail)
				return nil, err
			}
			s := t.UTC().Format(time.RFC3339)
			n.ResetsAt = &s
		}
		norm.Limits = append(norm.Limits, n)
	}
	return norm, nil
}

func normalizeSource(line string) (Source, *Error) {
	switch {
	case strings.HasPrefix(line, sourcePrefix+"your subscription "):
		return SourceSubscription, nil
	case strings.HasPrefix(line, sourcePrefix+"your overages "):
		return SourceOverage, nil
	}
	return "", &Error{Stage: StageNormalize, Reason: ReasonUnexpectedFormat,
		Detail: fmt.Sprintf("source: %q", line)}
}

// normalizeReset turns a reset time as printed into an absolute time.
//
// The printed form has no year, and when the reset is today it may have no
// date either. Of the candidates around the reading time (the previous,
// same and next year, or day), the one closest to the reading time is taken:
// a reset is never more than about a week away, so the others are months or
// a day off.
func normalizeReset(s string, at time.Time) (time.Time, *Error) {
	m := resetTime.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, &Error{Stage: StageNormalize, Reason: ReasonUnexpectedFormat,
			Detail: fmt.Sprintf("%q", s)}
	}
	monthText, dayText, hourText, minuteText, ampm, zone := m[1], m[2], m[3], m[4], m[5], m[6]
	if zone == "" {
		return time.Time{}, &Error{Stage: StageNormalize, Reason: ReasonNoTimeZone,
			Detail: fmt.Sprintf("%q", s)}
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, &Error{Stage: StageNormalize, Reason: ReasonUnknownTimeZone,
			Detail: fmt.Sprintf("%q: %v", s, err)}
	}

	hour, _ := strconv.Atoi(hourText)
	minute := 0
	if minuteText != "" {
		minute, _ = strconv.Atoi(minuteText)
	}
	if hour < 1 || hour > 12 || minute > 59 {
		return time.Time{}, &Error{Stage: StageNormalize, Reason: ReasonUnexpectedFormat,
			Detail: fmt.Sprintf("%q: no such time", s)}
	}
	hour %= 12
	if ampm == "pm" {
		hour += 12
	}

	local := at.In(loc)
	var candidates []time.Time
	if monthText == "" {
		for _, d := range []int{-1, 0, 1} {
			candidates = append(candidates,
				time.Date(local.Year(), local.Month(), local.Day()+d, hour, minute, 0, 0, loc))
		}
	} else {
		month, err := time.Parse("Jan", monthText)
		if err != nil {
			return time.Time{}, &Error{Stage: StageNormalize, Reason: ReasonUnexpectedFormat,
				Detail: fmt.Sprintf("%q: month", s)}
		}
		day, _ := strconv.Atoi(dayText)
		for _, y := range []int{local.Year() - 1, local.Year(), local.Year() + 1} {
			c := time.Date(y, month.Month(), day, hour, minute, 0, 0, loc)
			// time.Date rolls Feb 30 over into March; such a date is not
			// one that was printed.
			if c.Month() == month.Month() && c.Day() == day {
				candidates = append(candidates, c)
			}
		}
		if len(candidates) == 0 {
			return time.Time{}, &Error{Stage: StageNormalize, Reason: ReasonUnexpectedFormat,
				Detail: fmt.Sprintf("%q: no such date", s)}
		}
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if absDuration(c.Sub(at)) < absDuration(best.Sub(at)) {
			best = c
		}
	}
	return best, nil
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
